package e2e

import (
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/home-operations/tuppr/test/utils"
)

var _ = Describe("CRD Lifecycle", Ordered, func() {
	SetDefaultEventuallyTimeout(30 * time.Second)
	SetDefaultEventuallyPollingInterval(time.Second)

	Context("CRD Registration", func() {
		It("should have TalosUpgrade CRD registered", func() {
			By("verifying TalosUpgrade CRD exists")
			cmd := exec.Command("kubectl", "get", "crd", "talosupgrades.tuppr.home-operations.com")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "TalosUpgrade CRD should be registered")
		})

		It("should have KubernetesUpgrade CRD registered", func() {
			By("verifying KubernetesUpgrade CRD exists")
			cmd := exec.Command("kubectl", "get", "crd", "kubernetesupgrades.tuppr.home-operations.com")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "KubernetesUpgrade CRD should be registered")
		})

		It("should list both CRDs in API group", func() {
			By("checking API resources for tuppr group")
			cmd := exec.Command("kubectl", "api-resources", "--api-group=tuppr.home-operations.com")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(ContainSubstring("talosupgrades"))
			Expect(output).To(ContainSubstring("kubernetesupgrades"))
		})
	})

	Context("Controller Pod", func() {
		It("should stay running despite reconcile errors", func() {
			By("verifying controller pod remains running")
			Eventually(func(g Gomega) {
				cmd := exec.Command("kubectl", "get", "pods",
					"-l", "control-plane=controller-manager",
					"-n", namespace,
					"-o", "jsonpath={.items[0].status.phase}")
				output, err := utils.Run(cmd)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(output).To(Equal("Running"))
			}).Should(Succeed())

			By("waiting a few seconds and checking pod is still running")
			time.Sleep(5 * time.Second)

			cmd := exec.Command("kubectl", "get", "pods",
				"-l", "control-plane=controller-manager",
				"-n", namespace,
				"-o", "jsonpath={.items[0].status.phase}")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())
			Expect(output).To(Equal("Running"), "Controller pod should remain stable")
		})
	})

	Context("TalosUpgrade Lifecycle", func() {
		AfterEach(func() {
			By("cleaning up test TalosUpgrade resources")
			cmd := exec.Command("kubectl", "delete", "talosupgrade", "test-lifecycle", "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create and delete TalosUpgrade resource", func() {
			By("creating a TalosUpgrade")
			yaml := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: TalosUpgrade
metadata:
  name: test-lifecycle
spec:
  talos:
    version: v1.11.0
`
			err := utils.ApplyFromStdin(yaml)
			Expect(err).NotTo(HaveOccurred())

			By("verifying TalosUpgrade exists")
			Eventually(func(g Gomega) {
				exists := utils.ResourceExists("talosupgrade", "test-lifecycle")
				g.Expect(exists).To(BeTrue())
			}).Should(Succeed())

			By("verifying finalizer is added")
			Eventually(func(g Gomega) {
				finalizers, err := utils.GetResourceField("talosupgrade", "test-lifecycle", "{.metadata.finalizers}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(finalizers).To(ContainSubstring("talos-finalizer"))
			}).Should(Succeed())

			By("deleting TalosUpgrade")
			cmd := exec.Command("kubectl", "delete", "talosupgrade", "test-lifecycle")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying TalosUpgrade is eventually deleted")
			Eventually(func() bool {
				return !utils.ResourceExists("talosupgrade", "test-lifecycle")
			}, 60*time.Second).Should(BeTrue())
		})
	})

	Context("KubernetesUpgrade Lifecycle", func() {
		AfterEach(func() {
			By("cleaning up test KubernetesUpgrade resources")
			cmd := exec.Command("kubectl", "delete", "kubernetesupgrade", "test-k8s-lifecycle", "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		})

		It("should create and delete KubernetesUpgrade resource", func() {
			By("creating a KubernetesUpgrade")
			yaml := `
apiVersion: tuppr.home-operations.com/v1alpha1
kind: KubernetesUpgrade
metadata:
  name: test-k8s-lifecycle
spec:
  kubernetes:
    version: v1.34.0
`
			err := utils.ApplyFromStdin(yaml)
			Expect(err).NotTo(HaveOccurred())

			By("verifying KubernetesUpgrade exists")
			Eventually(func(g Gomega) {
				exists := utils.ResourceExists("kubernetesupgrade", "test-k8s-lifecycle")
				g.Expect(exists).To(BeTrue())
			}).Should(Succeed())

			By("verifying finalizer is added")
			Eventually(func(g Gomega) {
				finalizers, err := utils.GetResourceField("kubernetesupgrade", "test-k8s-lifecycle", "{.metadata.finalizers}")
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(finalizers).To(ContainSubstring("kubernetes-finalizer"))
			}).Should(Succeed())

			By("deleting KubernetesUpgrade")
			cmd := exec.Command("kubectl", "delete", "kubernetesupgrade", "test-k8s-lifecycle")
			_, err = utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			By("verifying KubernetesUpgrade is eventually deleted")
			Eventually(func() bool {
				return !utils.ResourceExists("kubernetesupgrade", "test-k8s-lifecycle")
			}, 60*time.Second).Should(BeTrue())
		})
	})
})
