//go:build e2e_hetzner

package e2ehetzner

import (
	"log"
	"time"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	talosUpgradeTimeout = 20 * time.Minute
	k8sUpgradeTimeout   = 15 * time.Minute
	phaseStartTimeout   = 2 * time.Minute
	phasePollInterval   = 10 * time.Second
	upgradePollInterval = 30 * time.Second
)

var _ = Describe("Upgrades", Ordered, func() {
	Describe("TalosUpgrade", func() {
		It("should upgrade all nodes to the target Talos version", func(ctx SpecContext) {
			By("Verifying all nodes are on the initial Talos version")
			versions, err := talosCluster.NodeVersions(ctx)
			Expect(err).NotTo(HaveOccurred())
			for ip, v := range versions {
				log.Printf("[talos-upgrade] node %s: %s", ip, v)
				Expect(v).To(Equal(cfg.TalosFromVersion), "node %s should be on %s", ip, cfg.TalosFromVersion)
			}

			By("Creating TalosUpgrade CR")
			upgrade := &tupprv1alpha1.TalosUpgrade{
				ObjectMeta: metav1.ObjectMeta{Name: "e2e-talos-upgrade"},
				Spec: tupprv1alpha1.TalosUpgradeSpec{
					Talos:  tupprv1alpha1.TalosSpec{Version: cfg.TalosToVersion},
					Policy: tupprv1alpha1.PolicySpec{Debug: true, RebootMode: "default"},
				},
			}
			Expect(k8sClient.Create(ctx, upgrade)).To(Succeed())

			By("Waiting for TalosUpgrade to start")
			Eventually(func(g Gomega) {
				var tu tupprv1alpha1.TalosUpgrade
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(upgrade), &tu)).To(Succeed())
				log.Printf("[talos-upgrade] phase: %s", tu.Status.Phase)
				g.Expect(tu.Status.Phase).To(Or(Equal("InProgress"), Equal("Completed")))
			}, phaseStartTimeout, phasePollInterval).Should(Succeed())

			By("Waiting for TalosUpgrade to complete")
			Eventually(func(g Gomega) {
				var tu tupprv1alpha1.TalosUpgrade
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(upgrade), &tu)).To(Succeed())
				log.Printf("[talos-upgrade] phase: %s", tu.Status.Phase)
				g.Expect(tu.Status.Phase).NotTo(Equal("Failed"), "upgrade entered Failed phase")
				g.Expect(tu.Status.Phase).To(Equal("Completed"))
			}, talosUpgradeTimeout, upgradePollInterval).Should(Succeed())

			By("Verifying all nodes are on the target Talos version")
			versions, err = talosCluster.NodeVersions(ctx)
			Expect(err).NotTo(HaveOccurred())
			for ip, v := range versions {
				log.Printf("[talos-upgrade] node %s: %s", ip, v)
				Expect(v).To(Equal(cfg.TalosToVersion), "node %s should be on %s", ip, cfg.TalosToVersion)
			}

			By("Verifying status details")
			var tu tupprv1alpha1.TalosUpgrade
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(upgrade), &tu)).To(Succeed())
			Expect(tu.Status.CompletedNodes).To(HaveLen(len(talosCluster.ServerIPs)), "all nodes should be completed")
			Expect(tu.Status.FailedNodes).To(BeEmpty(), "no nodes should have failed")

			log.Printf("[talos-upgrade] upgrade completed successfully")
		}, NodeTimeout(25*time.Minute))
	})

	Describe("KubernetesUpgrade", func() {
		It("should upgrade Kubernetes to the target version", func(ctx SpecContext) {
			By("Getting the current Kubernetes version from nodes")
			var nodes corev1.NodeList
			Expect(k8sClient.List(ctx, &nodes)).To(Succeed())
			for _, node := range nodes.Items {
				log.Printf("[k8s-upgrade] node %s kubelet: %s", node.Name, node.Status.NodeInfo.KubeletVersion)
			}

			By("Creating KubernetesUpgrade CR")
			upgrade := &tupprv1alpha1.KubernetesUpgrade{
				ObjectMeta: metav1.ObjectMeta{Name: "e2e-k8s-upgrade"},
				Spec: tupprv1alpha1.KubernetesUpgradeSpec{
					Kubernetes: tupprv1alpha1.KubernetesSpec{Version: cfg.K8sToVersion},
				},
			}
			Expect(k8sClient.Create(ctx, upgrade)).To(Succeed())

			By("Waiting for KubernetesUpgrade to start")
			Eventually(func(g Gomega) {
				var ku tupprv1alpha1.KubernetesUpgrade
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(upgrade), &ku)).To(Succeed())
				log.Printf("[k8s-upgrade] phase: %s", ku.Status.Phase)
				g.Expect(ku.Status.Phase).To(Or(Equal("InProgress"), Equal("Completed")))
			}, phaseStartTimeout, phasePollInterval).Should(Succeed())

			By("Waiting for KubernetesUpgrade to complete")
			Eventually(func(g Gomega) {
				var ku tupprv1alpha1.KubernetesUpgrade
				g.Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(upgrade), &ku)).To(Succeed())
				log.Printf("[k8s-upgrade] phase: %s", ku.Status.Phase)
				g.Expect(ku.Status.Phase).NotTo(Equal("Failed"), "upgrade entered Failed phase")
				g.Expect(ku.Status.Phase).To(Equal("Completed"))
			}, k8sUpgradeTimeout, upgradePollInterval).Should(Succeed())

			By("Verifying Kubernetes version on all nodes")
			Expect(k8sClient.List(ctx, &nodes)).To(Succeed())
			for _, node := range nodes.Items {
				log.Printf("[k8s-upgrade] node %s kubelet: %s", node.Name, node.Status.NodeInfo.KubeletVersion)
				Expect(node.Status.NodeInfo.KubeletVersion).To(Equal(cfg.K8sToVersion),
					"kubelet on %s should be %s", node.Name, cfg.K8sToVersion)
			}

			By("Verifying status details")
			var ku tupprv1alpha1.KubernetesUpgrade
			Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(upgrade), &ku)).To(Succeed())
			Expect(ku.Status.Phase).To(Equal("Completed"))
			Expect(ku.Status.LastError).To(BeEmpty(), "should have no error")

			log.Printf("[k8s-upgrade] upgrade completed successfully")
		}, NodeTimeout(20*time.Minute))
	})
})
