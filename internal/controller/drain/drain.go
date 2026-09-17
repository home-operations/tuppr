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
	storagev1 "k8s.io/api/storage/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const daemonSetKind = "DaemonSet"

// drainPollInterval is how often DrainNode re-checks the node for pods that
// are still terminating after eviction.
const drainPollInterval = 2 * time.Second

// Drainer handles node drain operations
type Drainer struct {
	client  client.Client
	skipPod *types.NamespacedName
}

// NewDrainer creates a new Drainer
func NewDrainer(c client.Client) *Drainer {
	return &Drainer{client: c}
}

// SkipPod excludes a specific pod from eviction, so a single-node drain doesn't
// evict the controller's own pod off the node it's draining.
func (d *Drainer) SkipPod(namespace, name string) *Drainer {
	if name != "" {
		d.skipPod = &types.NamespacedName{Namespace: namespace, Name: name}
	}
	return d
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

// DrainNode evicts the node's evictable pods and waits until they, and any pod
// that was already terminating, have left the node. Eviction only marks a pod
// for deletion; without the wait a reboot could cut pods off mid-termination.
func (d *Drainer) DrainNode(ctx context.Context, nodeName string, opts DrainOptions) error {
	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	pods, err := d.nodePods(ctx, nodeName, isDrainManaged)
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	for _, pod := range pods {
		if pod.DeletionTimestamp != nil {
			continue
		}
		if err := d.evictPod(ctx, &pod, opts); err != nil {
			return fmt.Errorf("failed to evict pod %s/%s: %w", pod.Namespace, pod.Name, err)
		}
	}

	// Only the pods seen above are waited for, so a pod that lands on the
	// cordoned node afterwards (a DaemonSet aside, only by bypassing the
	// scheduler) can't hold the drain open.
	pending := make(map[types.UID]struct{}, len(pods))
	for _, pod := range pods {
		pending[pod.UID] = struct{}{}
	}
	if err := wait.PollUntilContextCancel(ctx, drainPollInterval, true, func(ctx context.Context) (bool, error) {
		remaining, err := d.nodePods(ctx, nodeName, isDrainManaged)
		if err != nil {
			return false, err
		}
		for _, pod := range remaining {
			if _, ok := pending[pod.UID]; ok {
				return false, nil
			}
		}
		return true, nil
	}); err != nil {
		return fmt.Errorf("failed waiting for pods to terminate: %w", err)
	}

	return nil
}

// getEvictablePods returns pods that should be evicted from the node
func (d *Drainer) getEvictablePods(ctx context.Context, nodeName string) ([]corev1.Pod, error) {
	return d.nodePods(ctx, nodeName, shouldEvictPod)
}

// nodePods returns the node's pods that pass keep, excluding the controller's own pod.
func (d *Drainer) nodePods(ctx context.Context, nodeName string, keep func(*corev1.Pod) bool) ([]corev1.Pod, error) {
	var podList corev1.PodList
	if err := d.client.List(ctx, &podList, &client.ListOptions{
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": nodeName}),
	}); err != nil {
		return nil, err
	}

	var pods []corev1.Pod
	for _, pod := range podList.Items {
		if d.skipPod != nil && pod.Namespace == d.skipPod.Namespace && pod.Name == d.skipPod.Name {
			continue
		}
		if keep(&pod) {
			pods = append(pods, pod)
		}
	}

	return pods, nil
}

// shouldEvictPod determines if a pod should be evicted
func shouldEvictPod(pod *corev1.Pod) bool {
	// Skip pods that are already terminating
	if pod.DeletionTimestamp != nil {
		return false
	}

	return isDrainManaged(pod)
}

// isDrainManaged reports whether drain is responsible for getting the pod off
// the node. Terminating pods still count: they haven't left yet.
func isDrainManaged(pod *corev1.Pod) bool {
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
		if owner.Kind == daemonSetKind {
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

// EvictableVolumePVs returns the names of PersistentVolumes backing the PVCs of the
// node's evictable pods. Resolve these before draining, then pass to WaitForVolumeDetach.
func (d *Drainer) EvictableVolumePVs(ctx context.Context, nodeName string) ([]string, error) {
	pods, err := d.getEvictablePods(ctx, nodeName)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	var pvs []string
	for _, pod := range pods {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim == nil {
				continue
			}
			var pvc corev1.PersistentVolumeClaim
			if err := d.client.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: vol.PersistentVolumeClaim.ClaimName}, &pvc); err != nil {
				if apierrors.IsNotFound(err) {
					continue
				}
				return nil, fmt.Errorf("failed to get pvc %s/%s: %w", pod.Namespace, vol.PersistentVolumeClaim.ClaimName, err)
			}
			pvName := pvc.Spec.VolumeName
			if pvName == "" {
				continue
			}
			if _, ok := seen[pvName]; ok {
				continue
			}
			seen[pvName] = struct{}{}
			pvs = append(pvs, pvName)
		}
	}

	return pvs, nil
}

// WaitForVolumeDetach blocks until no VolumeAttachment on the node references any of
// the given PVs, or the timeout elapses — letting CSI teardown finish before the
// reboot so a stale attachment can't cause a Multi-Attach error elsewhere.
func (d *Drainer) WaitForVolumeDetach(ctx context.Context, nodeName string, pvNames []string, timeout, pollInterval time.Duration) error {
	if len(pvNames) == 0 {
		return nil
	}

	targets := make(map[string]struct{}, len(pvNames))
	for _, pv := range pvNames {
		targets[pv] = struct{}{}
	}

	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		var vaList storagev1.VolumeAttachmentList
		if err := d.client.List(ctx, &vaList); err != nil {
			return false, fmt.Errorf("failed to list volumeattachments: %w", err)
		}
		for _, va := range vaList.Items {
			if va.Spec.NodeName != nodeName {
				continue
			}
			if va.Spec.Source.PersistentVolumeName == nil {
				continue
			}
			if _, ok := targets[*va.Spec.Source.PersistentVolumeName]; ok {
				return false, nil
			}
		}
		return true, nil
	})
}
