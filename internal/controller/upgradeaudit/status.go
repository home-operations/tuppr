package upgradeaudit

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// PatchStatus injects observedGeneration (unless already set) and lastUpdated, then
// applies updates as a merge patch to the status subresource of obj.
func PatchStatus(ctx context.Context, c client.Client, obj client.Object, generation int64, updates map[string]any) error {
	if _, ok := updates["observedGeneration"]; !ok {
		updates["observedGeneration"] = generation
	}
	updates["lastUpdated"] = metav1.Now()

	patchBytes, err := json.Marshal(map[string]any{"status": updates})
	if err != nil {
		return fmt.Errorf("failed to marshal patch: %w", err)
	}

	return c.Status().Patch(ctx, obj, client.RawPatch(types.MergePatchType, patchBytes))
}
