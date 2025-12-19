/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	tupprv1alpha1 "github.com/home-operations/tuppr/api/v1alpha1"
)

var _ = Describe("KubernetesUpgrade Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		kubernetesupgrade := &tupprv1alpha1.KubernetesUpgrade{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind TalosUpgrade")
			err := k8sClient.Get(ctx, typeNamespacedName, kubernetesupgrade)
			if err != nil && errors.IsNotFound(err) {
				resource := &tupprv1alpha1.KubernetesUpgrade{
					ObjectMeta: metav1.ObjectMeta{
						Name: resourceName,
						// No namespace - TalosUpgrade is cluster-scoped
					},
					Spec: tupprv1alpha1.KubernetesUpgradeSpec{
						Kubernetes: tupprv1alpha1.KubernetesSpec{
							Version: "v1.34.0", // Required field with valid format
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &tupprv1alpha1.KubernetesUpgrade{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance KubernetesUpgrade")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &KubernetesUpgradeReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			// TODO(user): Add more specific assertions depending on your controller's reconciliation logic.
			// Example: If you expect a certain status condition after reconciliation, verify it here.
		})
	})

	Context("When finding controller nodes", func() {
		var reconciler *KubernetesUpgradeReconciler

		BeforeEach(func() {
			reconciler = &KubernetesUpgradeReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
		})

		It("should return appropriate error when no control plane nodes exist", func() {
			ctx := context.Background()

			// Ensure no nodes with control-plane label exist
			nodeList := &corev1.NodeList{}
			err := k8sClient.List(ctx, nodeList)
			Expect(err).NotTo(HaveOccurred())

			// Delete any existing control plane nodes for this test
			for _, node := range nodeList.Items {
				if _, hasLabel := node.Labels["node-role.kubernetes.io/control-plane"]; hasLabel {
					err := k8sClient.Delete(ctx, &node)
					Expect(err).NotTo(HaveOccurred())
				}
			}

			_, _, err = reconciler.findControllerNode(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("no controller node found with node-role.kubernetes.io/control-plane label"))
		})

		It("should return appropriate error when control plane nodes lack InternalIP", func() {
			ctx := context.Background()

			// Create a control plane node without InternalIP
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-control-plane-no-ip",
					Labels: map[string]string{
						"node-role.kubernetes.io/control-plane": "",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						// Intentionally no InternalIP
						{
							Type:    corev1.NodeHostName,
							Address: "test-control-plane-no-ip",
						},
					},
				},
			}

			err := k8sClient.Create(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			// Clean up after test
			defer func() {
				err := k8sClient.Delete(ctx, node)
				Expect(err).NotTo(HaveOccurred())
			}()

			_, _, err = reconciler.findControllerNode(ctx)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("found control plane nodes but failed to get internal IP"))
		})

		It("should successfully find control plane node with InternalIP", func() {
			ctx := context.Background()

			// Create a proper control plane node with InternalIP
			node := &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-control-plane-with-ip",
					Labels: map[string]string{
						"node-role.kubernetes.io/control-plane": "",
					},
				},
				Status: corev1.NodeStatus{
					Addresses: []corev1.NodeAddress{
						{
							Type:    corev1.NodeInternalIP,
							Address: "192.168.1.100",
						},
						{
							Type:    corev1.NodeHostName,
							Address: "test-control-plane-with-ip",
						},
					},
				},
			}

			err := k8sClient.Create(ctx, node)
			Expect(err).NotTo(HaveOccurred())

			// Clean up after test
			defer func() {
				err := k8sClient.Delete(ctx, node)
				Expect(err).NotTo(HaveOccurred())
			}()

			nodeName, nodeIP, err := reconciler.findControllerNode(ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(nodeName).To(Equal("test-control-plane-with-ip"))
			Expect(nodeIP).To(Equal("192.168.1.100"))
		})
	})
})
