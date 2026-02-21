package integration

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

var _ = Describe("KubernetesUpgrade Integration", func() {
	var (
		testNode    *corev1.Node
		cleanup     func()
		mockTalos   *mockTalosClient
		mockHealth  *mockHealthChecker
		mockVersion *mockVersionGetter
	)

	BeforeEach(func() {
		// Get the mocks from suite setup
		mockTalos = getMockTalosClient()
		mockTalos.Reset()
		mockHealth = getMockHealthChecker()
		mockVersion = getMockVersionGetter()

		testNode = createTestNode("k8s-test-node", "10.0.0.20")
		Expect(k8sClient.Create(ctx, testNode)).To(Succeed())

		// Ensure mocks are ready (prevents unused variable warnings)
		_ = mockTalos
		_ = mockHealth
		_ = mockVersion

		cleanup = func() {
			_ = k8sClient.Delete(ctx, testNode)
		}
	})

	AfterEach(func() {
		// Clean up any leftover Jobs from tests BEFORE deleting resources
		By("cleaning up any leftover Jobs")
		jobList := &batchv1.JobList{}
		_ = k8sClient.List(ctx, jobList)
		for i := range jobList.Items {
			job := &jobList.Items[i]
			// Force delete with propagation policy
			_ = k8sClient.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		}

		// Wait a moment for Jobs to be deleted
		time.Sleep(500 * time.Millisecond)

		if cleanup != nil {
			cleanup()
		}
	})

	Context("K8s upgrade lifecycle", func() {
		It("should handle a basic Kubernetes upgrade", func() {
			By("configuring talos version for the node")
			mockTalos.SetNodeVersion("10.0.0.20", "v1.33.0")

			By("creating a KubernetesUpgrade resource")
			k8sUpgrade := &tupprv1alpha1.KubernetesUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "k8s-test-upgrade",
				},
				Spec: tupprv1alpha1.KubernetesUpgradeSpec{
					Kubernetes: tupprv1alpha1.KubernetesSpec{
						Version: "v1.34.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, k8sUpgrade)).To(Succeed())

			By("verifying the upgrade job is created with the correct talosctl image")
			Eventually(func(g Gomega) {
				jobList := &batchv1.JobList{}
				g.Expect(k8sClient.List(ctx, jobList)).To(Succeed())
				g.Expect(jobList.Items).NotTo(BeEmpty())
				container := jobList.Items[0].Spec.Template.Spec.Containers[0]
				g.Expect(container.Image).To(Equal("ghcr.io/siderolabs/talosctl:v1.33.0"))
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, k8sUpgrade)).To(Succeed())
		})

		It("should set phase to Failed when talosctl version detection fails", func() {
			By("not configuring any talos version for the node (simulating API failure)")
			// mockTalos was Reset() in BeforeEach — GetNodeVersion will return an error

			By("creating a KubernetesUpgrade resource targeting a higher version")
			k8sUpgrade := &tupprv1alpha1.KubernetesUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "k8s-version-detect-fail",
				},
				Spec: tupprv1alpha1.KubernetesUpgradeSpec{
					Kubernetes: tupprv1alpha1.KubernetesSpec{
						Version: "v1.34.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, k8sUpgrade)).To(Succeed())

			By("verifying the upgrade phase is set to Failed")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "k8s-version-detect-fail"}, k8sUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(k8sUpgrade.Status.Phase).To(Equal(tupprv1alpha1.JobPhaseFailed))
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, k8sUpgrade)).To(Succeed())
		})
	})

	Context("Finalizer management", func() {
		It("should add finalizer on creation", func() {
			k8sUpgrade := &tupprv1alpha1.KubernetesUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "k8s-finalizer-test",
				},
				Spec: tupprv1alpha1.KubernetesUpgradeSpec{
					Kubernetes: tupprv1alpha1.KubernetesSpec{
						Version: "v1.34.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, k8sUpgrade)).To(Succeed())

			By("verifying finalizer is added")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "k8s-finalizer-test"}, k8sUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(k8sUpgrade.Finalizers).To(ContainElement("tuppr.home-operations.com/kubernetes-finalizer"))
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, k8sUpgrade)).To(Succeed())

			By("verifying resource is eventually deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "k8s-finalizer-test"}, k8sUpgrade)
				return err != nil
			}, 30*time.Second, 1*time.Second).Should(BeTrue())
		})
	})

	Context("ObservedGeneration tracking", func() {
		It("should track generation correctly", func() {
			k8sUpgrade := &tupprv1alpha1.KubernetesUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "k8s-gen-test",
				},
				Spec: tupprv1alpha1.KubernetesUpgradeSpec{
					Kubernetes: tupprv1alpha1.KubernetesSpec{
						Version: "v1.34.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, k8sUpgrade)).To(Succeed())

			By("verifying observedGeneration is set")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "k8s-gen-test"}, k8sUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(k8sUpgrade.Status.ObservedGeneration).To(Equal(k8sUpgrade.Generation))
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, k8sUpgrade)).To(Succeed())
		})
	})

	Context("Version check", func() {
		It("should handle version matching", func() {
			k8sUpgrade := &tupprv1alpha1.KubernetesUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "k8s-version-check",
				},
				Spec: tupprv1alpha1.KubernetesUpgradeSpec{
					Kubernetes: tupprv1alpha1.KubernetesSpec{
						Version: "v1.33.0", // Matches mock version
					},
				},
			}
			Expect(k8sClient.Create(ctx, k8sUpgrade)).To(Succeed())

			By("verifying upgrade is processed")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "k8s-version-check"}, k8sUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				// Version matches, so should complete or indicate no upgrade needed
				g.Expect(k8sUpgrade.Status.Phase).To(Or(
					Equal(tupprv1alpha1.JobPhaseCompleted),
					Equal(tupprv1alpha1.JobPhasePending),
				))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, k8sUpgrade)).To(Succeed())
		})
	})
})
