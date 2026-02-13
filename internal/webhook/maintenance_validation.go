package webhook

import (
	"errors"
	"time"

	"github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/home-operations/tuppr/internal/controller"
	cron "github.com/netresearch/go-cron"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

func validateMaintenanceWindows(spec *v1alpha1.MaintenanceSpec) (admission.Warnings, error) {
	if spec == nil || len(spec.Windows) == 0 {
		return nil, nil
	}
	var warnings admission.Warnings
	for _, window := range spec.Windows {
		warn, err := validateMaintenanceWindow(&window)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, warn...)
	}
	return warnings, nil
}

func validateMaintenanceWindow(window *v1alpha1.WindowSpec) (admission.Warnings, error) {
	var warnings admission.Warnings

	_, err := time.LoadLocation(window.Timezone)
	if err != nil {
		return nil, err
	}
	specParser := cron.NewParser(controller.CronjobDefaultOption)
	_, err = specParser.Parse(window.Start)
	if err != nil {
		return nil, err
	}
	if window.Duration.Duration <= 0 {
		return nil, errors.New("duration must be positive")
	}
	if window.Duration.Duration > 168*time.Hour {
		return nil, errors.New("duration must not exceed 7 days (168h)")
	}
	if window.Duration.Duration < time.Hour {
		warnings = append(warnings, "maintenance window duration < 1h: may not be enough time to complete upgrades")
	}
	return warnings, nil
}
