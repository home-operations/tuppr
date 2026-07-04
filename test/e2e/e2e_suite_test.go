package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/home-operations/tuppr/test/utils"
)

var (
	// projectImage is the name of the image which will be built and loaded
	// with the code source changes to be tested.
	projectImage = "example.com/tuppr:v0.0.1"
)

// TestE2E runs the end-to-end (e2e) test suite for the project against a Kind
// cluster: it builds the manager image, loads it into Kind, deploys the Helm
// chart, and exercises the controller + webhooks. Run it via `mise run
// test-e2e`, which provisions the cluster and sets KIND_CLUSTER; without that
// the suite skips, so a bare `go test ./...` stays green on machines without
// Docker/Kind.
func TestE2E(t *testing.T) {
	if os.Getenv("KIND_CLUSTER") == "" {
		t.Skip("KIND_CLUSTER not set; run via `mise run test-e2e` (requires Docker + Kind)")
	}
	RegisterFailHandler(Fail)
	// One suite-wide Eventually default: these settings are GLOBAL (the last
	// call wins for every suite), so per-file values would race under Ginkgo's
	// randomized container order. 3 minutes absorbs a fresh Kind node's
	// not-ready window plus image start.
	SetDefaultEventuallyTimeout(3 * time.Minute)
	SetDefaultEventuallyPollingInterval(time.Second)
	_, _ = fmt.Fprintf(GinkgoWriter, "Starting tuppr integration test suite\n")
	RunSpecs(t, "e2e suite")
}

// The whole deploy lives at suite level (not in a Describe's BeforeAll):
// Ginkgo randomizes top-level container order, so the webhook/lifecycle suites
// must find the controller already running whichever order they execute in.
var _ = BeforeSuite(func() {
	By("building the manager(Operator) image")
	// The Dockerfile's GO_VERSION build arg tracks the toolchain; derive it from
	// the running test binary so the image builds with the same Go.
	goVersion := strings.TrimPrefix(runtime.Version(), "go")
	cmd := exec.Command("docker", "build",
		"--build-arg", fmt.Sprintf("GO_VERSION=%s", goVersion),
		"-t", projectImage, ".")
	_, err := utils.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to build the manager(Operator) image")

	By("loading the manager(Operator) image on Kind")
	err = utils.LoadImageToKindClusterWithName(projectImage)
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "Failed to load the manager(Operator) image into Kind")

	By("creating manager namespace")
	cmd = exec.Command("kubectl", "create", "ns", namespace)
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to create namespace")

	By("labeling the namespace to enforce the restricted security policy")
	cmd = exec.Command("kubectl", "label", "--overwrite", "ns", namespace,
		"pod-security.kubernetes.io/enforce=restricted")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to label namespace with restricted policy")

	By("creating talosconfig secret")
	err = utils.CreateTalosConfigSecret(namespace)
	Expect(err).NotTo(HaveOccurred(), "Failed to create talosconfig secret")

	// The chart carries the CRDs in crds/, which helm installs natively — no
	// separate kubectl apply (it would conflict on field ownership).
	By("deploying the controller-manager via the Helm chart")
	cmd = exec.Command("helm", "install", "tuppr", "charts/tuppr",
		"--namespace", namespace,
		"--set", "image.repository=example.com/tuppr",
		"--set", "image.tag=v0.0.1",
		"--set", "talosServiceAccount.create=false")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "Failed to deploy the controller-manager")

	// Available = ready probes passing = /readyz green, which gates on the
	// webhook server having started — so specs can apply CRs immediately
	// without racing the webhook listener.
	By("waiting for the controller Deployment to become Available")
	cmd = exec.Command("kubectl", "-n", namespace, "wait", "--for=condition=Available",
		"deployment/tuppr", "--timeout=180s")
	_, err = utils.Run(cmd)
	Expect(err).NotTo(HaveOccurred(), "controller Deployment never became Available")
})

var _ = AfterSuite(func() {
	By("undeploying the controller-manager")
	cmd := exec.Command("helm", "uninstall", "tuppr", "--namespace", namespace)
	_, _ = utils.Run(cmd)

	By("uninstalling CRDs")
	cmd = exec.Command("kubectl", "delete", "--ignore-not-found", "-f", "charts/tuppr/crds")
	_, _ = utils.Run(cmd)

	By("removing manager namespace")
	cmd = exec.Command("kubectl", "delete", "ns", namespace)
	_, _ = utils.Run(cmd)
})
