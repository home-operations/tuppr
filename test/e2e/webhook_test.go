package e2e

import (
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/home-operations/tuppr/test/utils"
)

var _ = Describe("Webhook Validation", Ordered, func() {
	SetDefaultEventuallyTimeout(30 * time.Second)
	SetDefaultEventuallyPollingInterval(time.Second)

	BeforeAll(func() {
		By("waiting for webhook to be ready")
		Eventually(func(g Gomega) {
			cmd := exec.Command("kubectl", "get", "validatingwebhookconfiguration",
				"tuppr-validating-webhook-configuration",
				"-o", "jsonpath={.webhooks[0].clientConfig.caBundle}")
			output, err := utils.Run(cmd)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(output).NotTo(BeEmpty(), "Webhook caBundle should be populated")
		}, 2*time.Minute).Should(Succeed())
	})

	Context("TalosUpgrade Validation", func() {
		AfterEach(func() {
			By("cleaning up test resources")
			cmd := exec.Command("kubectl", "delete", "talosupgrade", "--all", "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			// Wait for resources to be fully deleted
			time.Sleep(2 * time.Second)
		})

		It("should accept valid TalosUpgrade", func() {
			yaml := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: valid-upgrade
spec:
  talos:
    version: v1.11.0
`
			err := utils.ApplyFromStdin(yaml)
			Expect(err).NotTo(HaveOccurred(), "Valid TalosUpgrade should be accepted")

			By("verifying resource was created")
			exists := utils.ResourceExists("talosupgrade", "valid-upgrade")
			Expect(exists).To(BeTrue())
		})

		It("should reject invalid version format", func() {
			yaml := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: invalid-version
spec:
  talos:
    version: 1.11.0
`
			err := utils.ApplyFromStdinExpectError(yaml)
			Expect(err).To(HaveOccurred(), "Invalid version format should be rejected")
			Expect(err.Error()).To(Or(
				ContainSubstring("does not match"),
				ContainSubstring("invalid"),
			))
		})

		It("should enforce singleton constraint", func() {
			By("creating first TalosUpgrade")
			yaml1 := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: first-upgrade
spec:
  talos:
    version: v1.11.0
`
			err := utils.ApplyFromStdin(yaml1)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for first resource to be created")
			Eventually(func() bool {
				return utils.ResourceExists("talosupgrade", "first-upgrade")
			}).Should(BeTrue())

			By("attempting to create second TalosUpgrade")
			yaml2 := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: second-upgrade
spec:
  talos:
    version: v1.12.0
`
			err = utils.ApplyFromStdinExpectError(yaml2)
			Expect(err).To(HaveOccurred(), "Second TalosUpgrade should be rejected")
			Expect(err.Error()).To(ContainSubstring("only one TalosUpgrade"))
		})

		It("should allow updating the same TalosUpgrade", func() {
			By("creating initial TalosUpgrade")
			yaml1 := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: update-test
spec:
  talos:
    version: v1.11.0
`
			err := utils.ApplyFromStdin(yaml1)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for resource to be created")
			Eventually(func() bool {
				return utils.ResourceExists("talosupgrade", "update-test")
			}).Should(BeTrue())

			By("updating the same resource")
			yaml2 := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: update-test
spec:
  talos:
    version: v1.12.0
`
			err = utils.ApplyFromStdin(yaml2)
			Expect(err).NotTo(HaveOccurred(), "Updating same resource should be allowed")
		})
	})

	Context("KubernetesUpgrade Validation", func() {
		AfterEach(func() {
			By("cleaning up test resources")
			cmd := exec.Command("kubectl", "delete", "kubernetesupgrade", "--all", "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
			// Wait for resources to be fully deleted
			time.Sleep(2 * time.Second)
		})

		It("should accept valid KubernetesUpgrade", func() {
			yaml := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: valid-k8s-upgrade
spec:
  kubernetes:
    version: v1.34.0
`
			err := utils.ApplyFromStdin(yaml)
			Expect(err).NotTo(HaveOccurred(), "Valid KubernetesUpgrade should be accepted")

			By("verifying resource was created")
			exists := utils.ResourceExists("kubernetesupgrade", "valid-k8s-upgrade")
			Expect(exists).To(BeTrue())
		})

		It("should reject invalid version format", func() {
			yaml := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: invalid-k8s-version
spec:
  kubernetes:
    version: 1.34.0
`
			err := utils.ApplyFromStdinExpectError(yaml)
			Expect(err).To(HaveOccurred(), "Invalid version format should be rejected")
			Expect(err.Error()).To(Or(
				ContainSubstring("does not match"),
				ContainSubstring("invalid"),
			))
		})

		It("should enforce singleton constraint", func() {
			By("creating first KubernetesUpgrade")
			yaml1 := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: first-k8s-upgrade
spec:
  kubernetes:
    version: v1.34.0
`
			err := utils.ApplyFromStdin(yaml1)
			Expect(err).NotTo(HaveOccurred())

			By("waiting for first resource to be created")
			Eventually(func() bool {
				return utils.ResourceExists("kubernetesupgrade", "first-k8s-upgrade")
			}).Should(BeTrue())

			By("attempting to create second KubernetesUpgrade")
			yaml2 := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: second-k8s-upgrade
spec:
  kubernetes:
    version: v1.35.0
`
			err = utils.ApplyFromStdinExpectError(yaml2)
			Expect(err).To(HaveOccurred(), "Second KubernetesUpgrade should be rejected")
			Expect(err.Error()).To(ContainSubstring("only one KubernetesUpgrade"))
		})
	})
})
