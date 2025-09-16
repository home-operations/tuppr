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

package webhook

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	talosclientconfig "github.com/siderolabs/talos/pkg/machinery/client/config"

	upgradev1alpha1 "github.com/home-operations/talup/api/v1alpha1"
)

const (
	TalosConfigSecretName = "talup"
	TalosConfigSecretKey  = "talosconfig"
)

// log is for logging in this package.
var taloslog = logf.Log.WithName("talos-resource")

// TalosPlanValidator validates Talos resources
type TalosPlanValidator struct {
	Client client.Client
}

//+kubebuilder:webhook:path=/validate-upgrade-home-operations-com-v1alpha1-talos,mutating=false,failurePolicy=fail,sideEffects=None,groups=upgrade.home-operations.com,resources=taloses,verbs=create;update,versions=v1alpha1,name=vtalos.kb.io,admissionReviewVersions=v1

var _ webhook.CustomValidator = &TalosPlanValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosPlanValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	talos := obj.(*upgradev1alpha1.TalosPlan)
	taloslog.Info("validate create", "name", talos.Name)

	return v.validateTalos(ctx, talos)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosPlanValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	talos := newObj.(*upgradev1alpha1.TalosPlan)
	taloslog.Info("validate update", "name", talos.Name)

	return v.validateTalos(ctx, talos)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type
func (v *TalosPlanValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	// No validation needed for delete
	return nil, nil
}

func (v *TalosPlanValidator) validateTalos(ctx context.Context, talos *upgradev1alpha1.TalosPlan) (admission.Warnings, error) {
	var warnings admission.Warnings

	// Validate that the Talos config secret exists
	secret := &corev1.Secret{}
	err := v.Client.Get(ctx, types.NamespacedName{
		Name:      TalosConfigSecretName,
		Namespace: talos.Namespace,
	}, secret)

	if err != nil {
		return warnings, fmt.Errorf("talosconfig secret '%s' not found in namespace '%s'. Please create this secret with your Talos configuration before creating TalosPlan resources: %w",
			TalosConfigSecretName, talos.Namespace, err)
	}

	// Validate that the secret has the required key
	configData, exists := secret.Data[TalosConfigSecretKey]
	if !exists {
		return warnings, fmt.Errorf("talosconfig secret '%s' missing required key '%s'. The secret must contain your talosconfig data under this key",
			TalosConfigSecretName, TalosConfigSecretKey)
	}

	// Add warning if secret data is empty
	if len(configData) == 0 {
		return warnings, fmt.Errorf("talosconfig secret data is empty. Please ensure the secret contains valid Talos configuration data")
	}

	// Validate that the talosconfig can be parsed
	_, err = talosclientconfig.FromBytes(configData)
	if err != nil {
		return warnings, fmt.Errorf("talosconfig in secret '%s' is invalid and cannot be parsed: %w. Please ensure the secret contains valid Talos configuration data",
			TalosConfigSecretName, err)
	}

	// Validate spec fields
	if err := v.validateTalosSpec(talos); err != nil {
		return warnings, fmt.Errorf("spec validation failed: %w", err)
	}

	return warnings, nil
}

func (v *TalosPlanValidator) validateTalosSpec(talos *upgradev1alpha1.TalosPlan) error {
	// Validate image repository and tag are not empty
	if talos.Spec.Image.Repository == "" {
		return fmt.Errorf("spec.image.repository cannot be empty")
	}

	if talos.Spec.Image.Tag == "" {
		return fmt.Errorf("spec.image.tag cannot be empty")
	}

	// Validate talosctl image
	if talos.Spec.Talosctl.Image.Repository == "" {
		return fmt.Errorf("spec.talosctl.image.repository cannot be empty")
	}

	if talos.Spec.Talosctl.Image.Tag == "" {
		return fmt.Errorf("spec.talosctl.image.tag cannot be empty")
	}

	// Validate timeout if provided
	if talos.Spec.Timeout != "" {
		// Try to parse the timeout
		if _, err := time.ParseDuration(talos.Spec.Timeout); err != nil {
			return fmt.Errorf("spec.timeout '%s' is not a valid duration: %w", talos.Spec.Timeout, err)
		}
	}

	// Validate reboot mode if provided
	if talos.Spec.RebootMode != "" {
		validModes := []string{"default", "powercycle"}
		isValid := false
		for _, mode := range validModes {
			if talos.Spec.RebootMode == mode {
				isValid = true
				break
			}
		}
		if !isValid {
			return fmt.Errorf("spec.rebootMode '%s' is invalid. Valid values are: %v", talos.Spec.RebootMode, validModes)
		}
	}

	// Validate maxRetries
	if talos.Spec.MaxRetries < 0 {
		return fmt.Errorf("spec.maxRetries cannot be negative")
	}

	return nil
}

func (v *TalosPlanValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&upgradev1alpha1.TalosPlan{}).
		WithValidator(v).
		Complete()
}
