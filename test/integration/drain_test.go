package integration

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

// forceCompleteJob forces a job to complete by directly updating its status
func forceCompleteJob(jobName string) {
	var job batchv1.Job
	for i := 0; i < 5; i++ {
		err := k8sClient.Get(ctx, types.NamespacedName{Name: jobName, Namespace: "tuppr-system"}, &job)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		now := metav1.Now()
		job.Status.Succeeded = 1
		job.Status.Active = 0
		job.Status.CompletionTime = &now
		job.Status.Conditions = []batchv1.JobCondition{
			{
				Type:               batchv1.JobSuccessCriteriaMet,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: now,
			},
			{
				Type:               batchv1.JobComplete,
				Status:             corev1.ConditionTrue,
				LastTransitionTime: now,
			},
		}

		err = k8sClient.Status().Update(ctx, &job)
		if err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

var _ = Describe("TalosUpgrade Drain Integration", func() {
	var (
		testNode  *corev1.Node
		testPod   *corev1.Pod
		cleanup   func()
		mockTalos *mockTalosClient
	)

	BeforeEach(func() {
		// Get the mocks from suite setup
		mockTalos = getMockTalosClient()
		mockTalos.Reset()

		testNode = createTestNode("drain-test-node", "10.0.0.20")
		Expect(k8sClient.Create(ctx, testNode)).To(Succeed())

		// Create a test pod on the node
		testPod = &corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "drain-test-pod",
				Namespace: "default",
			},
			Spec: corev1.PodSpec{
				NodeName: "drain-test-node",
				Containers: []corev1.Container{
					{
						Name:  "test",
						Image: "busybox",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, testPod)).To(Succeed())

		// Set up mock data for the test node
		mockTalos.SetNodeVersion("10.0.0.20", "v1.10.0") // Old version to trigger upgrade
		mockTalos.SetNodeInstallImage("10.0.0.20", "ghcr.io/siderolabs/installer:v1.10.0")

		cleanup = func() {
			_ = k8sClient.Delete(ctx, testPod)
			_ = k8sClient.Delete(ctx, testNode)
		}
	})

	AfterEach(func() {
		// Clean up any leftover Jobs from tests
		By("cleaning up any leftover Jobs")
		jobList := &batchv1.JobList{}
		_ = k8sClient.List(ctx, jobList)
		for i := range jobList.Items {
			job := &jobList.Items[i]
			_ = k8sClient.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground))
		}

		// Wait a moment for Jobs to be deleted
		time.Sleep(500 * time.Millisecond)

		if cleanup != nil {
			cleanup()
		}

		// Force delete the test pod if it still exists (it may have been evicted during drain)
		By("force deleting test pod if it still exists")
		pod := &corev1.Pod{}
		err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-test-pod", Namespace: "default"}, pod)
		if err == nil {
			// Pod still exists, force delete it
			_ = k8sClient.Delete(ctx, pod, client.GracePeriodSeconds(0))
		}

		// Also clean up any daemonset pod that might have been created
		dsPod := &corev1.Pod{}
		err = k8sClient.Get(ctx, types.NamespacedName{Name: "daemonset-pod", Namespace: "default"}, dsPod)
		if err == nil {
			_ = k8sClient.Delete(ctx, dsPod, client.GracePeriodSeconds(0))
		}

		// Wait for test pod to be fully deleted before next test
		By("waiting for test pod to be fully deleted")
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-test-pod", Namespace: "default"}, &corev1.Pod{})
			return err != nil
		}, 10*time.Second, 500*time.Millisecond).Should(BeTrue())
	})

	Context("Drain during upgrade", func() {
		It("should cordon node before upgrade when drain is enabled", func() {
			By("creating a TalosUpgrade with drain enabled")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "drain-upgrade-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
					Drain: &tupprv1alpha1.DrainSpec{
						Force: ptr.To(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("waiting for finalizer and initial status")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-upgrade-test"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Finalizers).To(ContainElement("tuppr.home-operations.com/talos-finalizer"))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("verifying node gets cordoned before upgrade job starts")
			Eventually(func(g Gomega) {
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-test-node"}, &node)
				g.Expect(err).NotTo(HaveOccurred())
				// Node should eventually be cordoned
				g.Expect(node.Spec.Unschedulable).To(BeTrue())
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})

		It("should cordon and uncordon node during upgrade", func() {
			By("creating a TalosUpgrade with drain enabled")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "drain-cordon-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
					Drain: &tupprv1alpha1.DrainSpec{
						Force: ptr.To(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("waiting for finalizer and initial status")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-cordon-test"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Finalizers).To(ContainElement("tuppr.home-operations.com/talos-finalizer"))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("waiting for job to be created and drain to occur")
			Eventually(func(g Gomega) {
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-test-node"}, &node)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(node.Spec.Unschedulable).To(BeTrue())
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("simulating node version update to match target")
			mockTalos.SetNodeVersion("10.0.0.20", "v1.11.0")

			// Wait for the job to be created first
			var jobName string
			Eventually(func(g Gomega) {
				jobList := &batchv1.JobList{}
				err := k8sClient.List(ctx, jobList, client.InNamespace("tuppr-system"))
				g.Expect(err).NotTo(HaveOccurred())
				for _, job := range jobList.Items {
					if job.Labels["tuppr.home-operations.com/target-node"] == "drain-test-node" {
						jobName = job.Name
						break
					}
				}
				g.Expect(jobName).NotTo(BeEmpty())
			}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

			By("completing the upgrade job")
			forceCompleteJob(jobName)

			By("waiting for node to be uncordoned after upgrade")
			Eventually(func(g Gomega) {
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-test-node"}, &node)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(node.Spec.Unschedulable).To(BeFalse())
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})

		It("should respect disableEviction setting", func() {
			By("creating a TalosUpgrade with disableEviction enabled")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "drain-no-eviction-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
					Drain: &tupprv1alpha1.DrainSpec{
						DisableEviction: ptr.To(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("waiting for finalizer and initial status")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-no-eviction-test"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Finalizers).To(ContainElement("tuppr.home-operations.com/talos-finalizer"))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("verifying node gets cordoned")
			Eventually(func(g Gomega) {
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-test-node"}, &node)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(node.Spec.Unschedulable).To(BeTrue())
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})
	})

	Context("Drain without spec", func() {
		It("should not cordon node when drain spec is not set", func() {
			By("creating a TalosUpgrade without drain spec")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "no-drain-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
					// No Drain spec set
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("waiting for finalizer and initial status")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "no-drain-test"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Finalizers).To(ContainElement("tuppr.home-operations.com/talos-finalizer"))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("waiting for job to be created")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "no-drain-test"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Status.CurrentNode).To(Equal("drain-test-node"))
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("verifying node is NOT cordoned (remains schedulable)")
			var node corev1.Node
			err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-test-node"}, &node)
			Expect(err).NotTo(HaveOccurred())
			Expect(node.Spec.Unschedulable).To(BeFalse())

			By("cleaning up")
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})
	})

	Context("Drain with DaemonSet pods", func() {
		It("should handle DaemonSet pods correctly", func() {
			By("creating a DaemonSet pod on the node")
			daemonSetPod := &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "daemonset-pod",
					Namespace: "default",
					OwnerReferences: []metav1.OwnerReference{
						{
							APIVersion: "apps/v1",
							Kind:       "DaemonSet",
							Name:       "test-daemonset",
							UID:        "test-ds-uid-12345",
						},
					},
				},
				Spec: corev1.PodSpec{
					NodeName: "drain-test-node",
					Containers: []corev1.Container{
						{
							Name:  "test",
							Image: "busybox",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, daemonSetPod)).To(Succeed())

			By("creating a TalosUpgrade with drain enabled")
			talosUpgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{
					Name: "drain-daemonset-test",
				},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos: tupprv1alpha1.TalosSpec{
						Version: "v1.11.0",
					},
					Drain: &tupprv1alpha1.DrainSpec{
						Force: ptr.To(true),
					},
				},
			}
			Expect(k8sClient.Create(ctx, talosUpgrade)).To(Succeed())

			By("waiting for finalizer and initial status")
			Eventually(func(g Gomega) {
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-daemonset-test"}, talosUpgrade)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(talosUpgrade.Finalizers).To(ContainElement("tuppr.home-operations.com/talos-finalizer"))
			}, 15*time.Second, 500*time.Millisecond).Should(Succeed())

			By("waiting for job to be created and drain to occur")
			Eventually(func(g Gomega) {
				var node corev1.Node
				err := k8sClient.Get(ctx, types.NamespacedName{Name: "drain-test-node"}, &node)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(node.Spec.Unschedulable).To(BeTrue())
			}, 30*time.Second, 1*time.Second).Should(Succeed())

			By("cleaning up")
			_ = k8sClient.Delete(ctx, daemonSetPod)
			Expect(k8sClient.Delete(ctx, talosUpgrade)).To(Succeed())
		})
	})
})
