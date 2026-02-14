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

This code was originally written by github.com/uozalp/kangal-patch
and has been adapted for use in this project.
*/

package drain

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// Drainer handles node drain operations
type Drainer struct {
	client client.Client
}

// NewDrainer creates a new Drainer
func NewDrainer(c client.Client) *Drainer {
	return &Drainer{client: c}
}

// CordonNode marks a node as unschedulable
func (d *Drainer) CordonNode(ctx context.Context, nodeName string) error {
	var node corev1.Node
	if err := d.client.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	if node.Spec.Unschedulable {
		return nil
	}

	node.Spec.Unschedulable = true
	if err := d.client.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to cordon node: %w", err)
	}

	return nil
}

// UncordonNode marks a node as schedulable
func (d *Drainer) UncordonNode(ctx context.Context, nodeName string) error {
	var node corev1.Node
	if err := d.client.Get(ctx, types.NamespacedName{Name: nodeName}, &node); err != nil {
		return fmt.Errorf("failed to get node: %w", err)
	}

	if !node.Spec.Unschedulable {
		return nil
	}

	node.Spec.Unschedulable = false
	if err := d.client.Update(ctx, &node); err != nil {
		return fmt.Errorf("failed to uncordon node: %w", err)
	}

	return nil
}

// DrainOptions configures drain behavior
type DrainOptions struct {
	// RespectPDBs indicates whether to respect PodDisruptionBudgets
	RespectPDBs bool
	// Timeout is the maximum time to wait for drain
	Timeout time.Duration
	// GracePeriod is the grace period for pod termination
	GracePeriod *int64
}

// DrainNode evicts all pods from a node
func (d *Drainer) DrainNode(ctx context.Context, nodeName string, opts DrainOptions) error {
	pods, err := d.getEvictablePods(ctx, nodeName)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	for _, pod := range pods {
		if err := d.evictPod(ctx, &pod, opts); err != nil {
			return fmt.Errorf("failed to evict pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}

	return nil
}

// getEvictablePods returns pods that should be evicted from the node
func (d *Drainer) getEvictablePods(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
	var podList corev1.PodList
	if err := d.client.List(ctx, &podList, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
	}); err != nil {
		return nil, err
	}

	var evictable []corev1.Pod
	for _, pod := range podList.Items {
		if shouldEvictPod(&pod) {
			evictable = append(evictable, pod)
		}
	}

	return evictable, nil
}

// shouldEvictPod determines if a pod should be evicted
func shouldEvictPod(pod *corev1.Pod) bool {
	// Skip pods that are already terminating
	if pod.DeletionTimestamp != nil {
		return false
	}

	// Skip completed/failed pods
	if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
		return false
	}

	// Skip DaemonSet pods
	if isDaemonSetPod(pod) {
		return false
	}

	// Skip mirror pods (static pods)
	if isMirrorPod(pod) {
		return false
	}

	return true
}

// isDaemonSetPod checks if a pod is managed by a DaemonSet
func isDaemonSetPod(pod *corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}

// isMirrorPod checks if a pod is a mirror pod (static pod)
func isMirrorPod(pod *corev1.Pod) bool {
	_, ok := pod.Annotations[corev1.MirrorPodAnnotationKey]
	return ok
}

// evictPod evicts a single pod
func (d *Drainer) evictPod(ctx context.Context, pod *corev1.Pod, opts DrainOptions) error {
	eviction := &policyv1.Eviction{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pod.Name,
			Namespace: pod.Namespace,
		},
	}

	if opts.GracePeriod != nil {
		eviction.DeleteOptions = &metav1.DeleteOptions{
			GracePeriodSeconds: opts.GracePeriod,
		}
	}

	if err := d.client.SubResource("eviction").Create(ctx, pod, eviction); err != nil {
		if !opts.RespectPDBs && apierrors.IsTooManyRequests(err) {
			return d.deletePod(ctx, pod, opts.GracePeriod)
		}
		return err
	}

	return nil
}

// deletePod forcefully deletes a pod when eviction is blocked by PDB
func (d *Drainer) deletePod(ctx context.Context, pod *corev1.Pod, gracePeriod *int64) error {
	deleteOpts := &client.DeleteOptions{}
	if gracePeriod != nil {
		deleteOpts.GracePeriodSeconds = gracePeriod
	}

	if err := d.client.Delete(ctx, pod, deleteOpts); err != nil {
		return fmt.Errorf("failed to delete pod: %w", err)
	}

	return nil
}

// IsDrained checks if all evictable pods have been removed from the node
func (d *Drainer) IsDrained(ctx context.Context, nodeName string) (bool, error) {
	pods, err := d.getEvictablePods(ctx, nodeName)
	if err != nil {
		return false, err
	}
	return len(pods) == 0, nil
}
