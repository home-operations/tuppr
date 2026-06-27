package drain

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testDsName = "test-ds"

func newTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	_ = policyv1.AddToScheme(s)
	_ = storagev1.AddToScheme(s)
	return s
}

// nodeNameIndex registers the spec.nodeName field index getEvictablePods relies on.
func nodeNameIndex(b *fake.ClientBuilder) *fake.ClientBuilder {
	return b.WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
		return []string{obj.(*corev1.Pod).Spec.NodeName}
	})
}

func podWithPVC(name, nodeName, claimName string) *corev1.Pod { //nolint:unparam
	p := newPod(name, "default", nodeName, corev1.PodRunning, nil, nil)
	p.Spec.Volumes = []corev1.Volume{{
		Name: "data",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: claimName},
		},
	}}
	return p
}

func pvc(name, pvName string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "default"},
		Spec:       corev1.PersistentVolumeClaimSpec{VolumeName: pvName},
	}
}

func volumeAttachment(name, nodeName, pvName string) *storagev1.VolumeAttachment {
	return &storagev1.VolumeAttachment{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec: storagev1.VolumeAttachmentSpec{
			NodeName: nodeName,
			Source:   storagev1.VolumeAttachmentSource{PersistentVolumeName: ptr.To(pvName)},
		},
	}
}

func newNode(name string, unschedulable bool) *corev1.Node { //nolint:unparam
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Spec: corev1.NodeSpec{
			Unschedulable: unschedulable,
		},
	}
}

func newPod(name, namespace, nodeName string, phase corev1.PodPhase, ownerRefs []metav1.OwnerReference, annotations map[string]string) *corev1.Pod { //nolint:unparam
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       namespace,
			OwnerReferences: ownerRefs,
			Annotations:     annotations,
		},
		Spec: corev1.PodSpec{
			NodeName: nodeName,
		},
		Status: corev1.PodStatus{
			Phase: phase,
		},
	}
}

func TestCordonNode(t *testing.T) {
	tests := []struct {
		name          string
		unschedulable bool
		wantErr       bool
	}{
		{
			name:          "cordons schedulable node",
			unschedulable: false,
			wantErr:       false,
		},
		{
			name:          "skips already cordoned node",
			unschedulable: true,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			node := newNode("test-node", tt.unschedulable)
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
			drainer := NewDrainer(cl)

			err := drainer.CordonNode(context.Background(), "test-node")
			if (err != nil) != tt.wantErr {
				t.Fatalf("CordonNode() error = %v, wantErr %v", err, tt.wantErr)
			}

			var updated corev1.Node
			if err := cl.Get(context.Background(), client.ObjectKey{Name: "test-node"}, &updated); err != nil {
				t.Fatalf("failed to get node: %v", err)
			}

			if !updated.Spec.Unschedulable {
				t.Fatal("expected node to be unschedulable")
			}
		})
	}
}

func TestUncordonNode(t *testing.T) {
	tests := []struct {
		name          string
		unschedulable bool
		wantErr       bool
	}{
		{
			name:          "uncordons cordoned node",
			unschedulable: true,
			wantErr:       false,
		},
		{
			name:          "skips already schedulable node",
			unschedulable: false,
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme := newTestScheme()
			node := newNode("test-node", tt.unschedulable)
			cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(node).Build()
			drainer := NewDrainer(cl)

			err := drainer.UncordonNode(context.Background(), "test-node")
			if (err != nil) != tt.wantErr {
				t.Fatalf("UncordonNode() error = %v, wantErr %v", err, tt.wantErr)
			}

			var updated corev1.Node
			if err := cl.Get(context.Background(), client.ObjectKey{Name: "test-node"}, &updated); err != nil {
				t.Fatalf("failed to get node: %v", err)
			}

			if updated.Spec.Unschedulable {
				t.Fatal("expected node to be schedulable")
			}
		})
	}
}

func TestShouldEvictPod(t *testing.T) {
	tests := []struct {
		name     string
		pod      *corev1.Pod
		expected bool
	}{
		{
			name:     "running pod should be evicted",
			pod:      newPod("test-pod", "default", "test-node", corev1.PodRunning, nil, nil),
			expected: true,
		},
		{
			name:     "pending pod should be evicted",
			pod:      newPod("test-pod", "default", "test-node", corev1.PodPending, nil, nil),
			expected: true,
		},
		{
			name:     "succeeded pod should not be evicted",
			pod:      newPod("test-pod", "default", "test-node", corev1.PodSucceeded, nil, nil),
			expected: false,
		},
		{
			name:     "failed pod should not be evicted",
			pod:      newPod("test-pod", "default", "test-node", corev1.PodFailed, nil, nil),
			expected: false,
		},
		{
			name: "daemonset pod should not be evicted",
			pod: newPod("test-pod", "default", "test-node", corev1.PodRunning, []metav1.OwnerReference{
				{Kind: daemonSetKind, Name: testDsName},
			}, nil),
			expected: false,
		},
		{
			name: "mirror pod should not be evicted",
			pod: newPod("test-pod", "default", "test-node", corev1.PodRunning, nil, map[string]string{
				corev1.MirrorPodAnnotationKey: "true",
			}),
			expected: false,
		},
		{
			name: "terminating pod should not be evicted",
			pod: func() *corev1.Pod {
				p := newPod("test-pod", "default", "test-node", corev1.PodRunning, nil, nil)
				now := metav1.Now()
				p.DeletionTimestamp = &now
				return p
			}(),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldEvictPod(tt.pod)
			if result != tt.expected {
				t.Fatalf("shouldEvictPod() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsDaemonSetPod(t *testing.T) {
	tests := []struct {
		name      string
		ownerRefs []metav1.OwnerReference
		expected  bool
	}{
		{
			name:      "no owner refs",
			ownerRefs: nil,
			expected:  false,
		},
		{
			name: "daemonset owner",
			ownerRefs: []metav1.OwnerReference{
				{Kind: daemonSetKind, Name: testDsName},
			},
			expected: true,
		},
		{
			name: "replicaset owner",
			ownerRefs: []metav1.OwnerReference{
				{Kind: "ReplicaSet", Name: "test-rs"},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := newPod("test", "default", "node", corev1.PodRunning, tt.ownerRefs, nil)
			result := isDaemonSetPod(pod)
			if result != tt.expected {
				t.Fatalf("isDaemonSetPod() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestIsMirrorPod(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		expected    bool
	}{
		{
			name:        "no annotations",
			annotations: nil,
			expected:    false,
		},
		{
			name: "has mirror pod annotation",
			annotations: map[string]string{
				corev1.MirrorPodAnnotationKey: "true",
			},
			expected: true,
		},
		{
			name: "other annotations",
			annotations: map[string]string{
				"other": "value",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pod := newPod("test", "default", "node", corev1.PodRunning, nil, tt.annotations)
			result := isMirrorPod(pod)
			if result != tt.expected {
				t.Fatalf("isMirrorPod() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetEvictablePods(t *testing.T) {
	scheme := newTestScheme()
	node := newNode("test-node", false)

	// Create various pods
	runningPod := newPod("running-pod", "default", "test-node", corev1.PodRunning, nil, nil)
	succeededPod := newPod("succeeded-pod", "default", "test-node", corev1.PodSucceeded, nil, nil)
	daemonSetPod := newPod("daemonset-pod", "default", "test-node", corev1.PodRunning, []metav1.OwnerReference{
		{Kind: daemonSetKind, Name: testDsName},
	}, nil)
	onOtherNode := newPod("other-node-pod", "default", "other-node", corev1.PodRunning, nil, nil)

	// Create client with field indexer for spec.nodeName
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			return []string{obj.(*corev1.Pod).Spec.NodeName}
		}).
		WithObjects(node, runningPod, succeededPod, daemonSetPod, onOtherNode).Build()
	drainer := NewDrainer(cl)

	pods, err := drainer.getEvictablePods(context.Background(), "test-node")
	if err != nil {
		t.Fatalf("getEvictablePods() error = %v", err)
	}

	if len(pods) != 1 {
		t.Fatalf("expected 1 evictable pod, got %d", len(pods))
	}

	if pods[0].Name != "running-pod" {
		t.Fatalf("expected running-pod, got %s", pods[0].Name)
	}
}

func TestGetEvictablePods_SkipsControllerPod(t *testing.T) {
	scheme := newTestScheme()
	node := newNode("test-node", false)

	controllerPod := newPod("tuppr-controller", "tuppr-system", "test-node", corev1.PodRunning, nil, nil)
	workloadPod := newPod("workload", "default", "test-node", corev1.PodRunning, nil, nil)

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
			return []string{obj.(*corev1.Pod).Spec.NodeName}
		}).
		WithObjects(node, controllerPod, workloadPod).Build()

	drainer := NewDrainer(cl).SkipPod("tuppr-system", "tuppr-controller")

	pods, err := drainer.getEvictablePods(context.Background(), "test-node")
	if err != nil {
		t.Fatalf("getEvictablePods() error = %v", err)
	}

	if len(pods) != 1 {
		t.Fatalf("expected 1 evictable pod (controller skipped), got %d", len(pods))
	}
	if pods[0].Name != "workload" {
		t.Fatalf("expected workload pod, got %s", pods[0].Name)
	}
}

func TestIsDrained(t *testing.T) {
	scheme := newTestScheme()

	tests := []struct {
		name     string
		pods     []*corev1.Pod
		expected bool
	}{
		{
			name:     "no pods",
			pods:     nil,
			expected: true,
		},
		{
			name: "only daemonset pods",
			pods: []*corev1.Pod{
				newPod("ds-pod", "default", "test-node", corev1.PodRunning, []metav1.OwnerReference{
					{Kind: daemonSetKind, Name: testDsName},
				}, nil),
			},
			expected: true,
		},
		{
			name: "has evictable pod",
			pods: []*corev1.Pod{
				newPod("evictable", "default", "test-node", corev1.PodRunning, nil, nil),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := newNode("test-node", false)
			objs := []client.Object{node}
			for _, p := range tt.pods {
				objs = append(objs, p)
			}

			// Create client with field indexer for spec.nodeName
			cl := fake.NewClientBuilder().WithScheme(scheme).
				WithIndex(&corev1.Pod{}, "spec.nodeName", func(obj client.Object) []string {
					return []string{obj.(*corev1.Pod).Spec.NodeName}
				}).
				WithObjects(objs...).Build()
			drainer := NewDrainer(cl)

			result, err := drainer.IsDrained(context.Background(), "test-node")
			if err != nil {
				t.Fatalf("IsDrained() error = %v", err)
			}
			if result != tt.expected {
				t.Fatalf("IsDrained() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestEvictableVolumePVs(t *testing.T) {
	scheme := newTestScheme()
	node := newNode("test-node", false)

	// Two evictable pods sharing pv-shared, plus a bound pv-1 and an unbound claim.
	podA := podWithPVC("pod-a", "test-node", "claim-1")
	podB := podWithPVC("pod-b", "test-node", "claim-shared")
	podC := podWithPVC("pod-c", "test-node", "claim-shared") // dedup with podB
	podUnbound := podWithPVC("pod-unbound", "test-node", "claim-unbound")

	// DaemonSet pod is not evictable; its volume must not be waited on.
	dsPod := podWithPVC("ds-pod", "test-node", "claim-ds")
	dsPod.OwnerReferences = []metav1.OwnerReference{{Kind: daemonSetKind, Name: testDsName}}

	cl := nodeNameIndex(fake.NewClientBuilder().WithScheme(scheme)).
		WithObjects(node, podA, podB, podC, podUnbound, dsPod,
			pvc("claim-1", "pv-1"),
			pvc("claim-shared", "pv-shared"),
			pvc("claim-unbound", ""), // unbound: no VolumeName
			pvc("claim-ds", "pv-ds"),
		).Build()
	drainer := NewDrainer(cl)

	pvs, err := drainer.EvictableVolumePVs(context.Background(), "test-node")
	if err != nil {
		t.Fatalf("EvictableVolumePVs() error = %v", err)
	}

	got := map[string]bool{}
	for _, pv := range pvs {
		got[pv] = true
	}
	if len(pvs) != 2 || !got["pv-1"] || !got["pv-shared"] {
		t.Fatalf("expected [pv-1 pv-shared], got %v", pvs)
	}
}

func TestWaitForVolumeDetach(t *testing.T) {
	scheme := newTestScheme()
	const (
		node = "test-node"
		pv   = "pv-1"
	)

	t.Run("no target PVs returns immediately", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		if err := NewDrainer(cl).WaitForVolumeDetach(context.Background(), node, nil, time.Second, 10*time.Millisecond); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("no matching attachment returns immediately", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			volumeAttachment("va-other-node", "another-node", pv),
			volumeAttachment("va-other-pv", node, "pv-2"),
		).Build()
		if err := NewDrainer(cl).WaitForVolumeDetach(context.Background(), node, []string{pv}, time.Second, 10*time.Millisecond); err != nil {
			t.Fatalf("expected nil, got %v", err)
		}
	})

	t.Run("lingering attachment times out", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(
			volumeAttachment("va-1", node, pv),
		).Build()
		err := NewDrainer(cl).WaitForVolumeDetach(context.Background(), node, []string{pv}, 100*time.Millisecond, 10*time.Millisecond)
		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}
	})

	t.Run("returns once attachment is deleted", func(t *testing.T) {
		va := volumeAttachment("va-1", node, pv)
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(va).Build()
		drainer := NewDrainer(cl)

		go func() {
			time.Sleep(30 * time.Millisecond)
			_ = cl.Delete(context.Background(), va)
		}()

		if err := drainer.WaitForVolumeDetach(context.Background(), node, []string{pv}, 2*time.Second, 10*time.Millisecond); err != nil {
			t.Fatalf("expected nil once detached, got %v", err)
		}
	})
}

func TestDrainOptions(t *testing.T) {
	opts := DrainOptions{
		RespectPDBs: true,
		Timeout:     5 * time.Minute,
		GracePeriod: ptr.To(int64(30)),
	}

	if !opts.RespectPDBs {
		t.Fatal("expected RespectPDBs to be true")
	}

	if opts.Timeout != 5*time.Minute {
		t.Fatalf("expected timeout 5m, got %v", opts.Timeout)
	}

	if opts.GracePeriod == nil || *opts.GracePeriod != 30 {
		t.Fatal("expected grace period 30")
	}
}
