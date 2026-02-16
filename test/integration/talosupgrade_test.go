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

var _ = Describe("TalosUpgrade Integration", func() {
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

		testNode = createTestNode("test-node", "10.0.0.10")
		Expect(k8sClient.Create(ctx, testNode)).To(Succeed())

		// Set up mock data for the test node
		mockTalos.SetNodeVersion("10.0.0.10", "v1.10.0") // Old version to trigger upgrade
		mockTalos.SetNodeInstallImage("10.0.0.10", "ghcr.io/siderolabs/installer:v1.10.0")

		// Ensure mocks are ready (prevents unused variable warnings)
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

	Context("Single node upgrade", func() {
		It("should process a TalosUpgrade and add finalizer", func() {
			By("creating a TalosUpgrade resource")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-upgrade",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("verifying finalizer is added and status is updated")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-upgrade"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Finalizers).To(ContainElement("tuppr.home-operations.com/talos-finalizer"))
				g.Expect(talosUpgrade.Status.Phase).NotTo(BeEmpty())
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})
	})

	Context("Generation tracking", func() {
		It("should update observedGeneration", func() {
			By("creating initial upgrade")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "gen-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("waiting for observedGeneration to be set")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "gen-test"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Status.ObservedGeneration).To(BeNumerically(">", 0))
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})
	})

	Context("Finalizer management", func() {
		It("should add finalizer on creation", func() {
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "finalizer-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("verifying finalizer is added")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "finalizer-test"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Finalizers).To(ContainElement("tuppr.home-operations.com/talos-finalizer"))
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())

			By("verifying resource is eventually deleted")
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "finalizer-test"}, talosUpgrade)
				return err != nil
			}, 30*time.Second, 1*time.Second).Should(BeTrue())
		})
	})

	Context("Node labeling", func() {
		It("should add upgrading label when job is created", func() {
			By("creating a TalosUpgrade resource")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "label-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("waiting for upgrade job to be created")
			Eventually(func(g Gomega) {
				jobList := &batchv1.JobList{}
				err := k8sClient.List(ctx, jobList, client.MatchingLabels{"app.kubernetes.io/name": "talos-upgrade"})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(jobList.Items).To(HaveLen(1))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("verifying node has upgrading label")
			Eventually(func(g Gomega) {
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-node"}, &node)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(node.Labels).To(HaveKey("tuppr.home-operations.com/upgrading"))
				g.Expect(node.Labels["tuppr.home-operations.com/upgrading"]).To(Equal("true"))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})

		It("should remove upgrading label when job succeeds", func() {
			By("creating a TalosUpgrade resource")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "label-removal-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("waiting for node to get upgrading label")
			Eventually(func(g Gomega) {
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-node"}, &node)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(node.Labels["tuppr.home-operations.com/upgrading"]).To(Equal("true"))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("marking job as succeeded")
			var job batchv1.Job
			Eventually(func(g Gomega) {
				jobList := &batchv1.JobList{}
				err := k8sClient.List(ctx, jobList, client.MatchingLabels{"app.kubernetes.io/name": "talos-upgrade"})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(jobList.Items).To(HaveLen(1))
				job = jobList.Items[0]
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			Eventually(func(g Gomega) {
				jobList := &batchv1.JobList{}
				err := k8sClient.List(ctx, jobList, client.MatchingLabels{"app.kubernetes.io/name": "talos-upgrade"})
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(jobList.Items).To(HaveLen(1))
				job = jobList.Items[0]
				job.Status.Succeeded = 1
				g.Expect(k8sClient.Status().Update(ctx, &job)).To(Succeed())
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			mockTalos.SetNodeVersion("10.0.0.10", "v1.11.0")

			By("verifying upgrading label is removed")
			Eventually(func(g Gomega) {
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "test-node"}, &node)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(node.Labels).NotTo(HaveKey("tuppr.home-operations.com/upgrading"))
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})
	})
})
