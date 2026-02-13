package controller

import (
	"time"

	"github.com/home-operations/tuppr/api/v1alpha1"
	cron "github.com/netresearch/go-cron"
)

var CronjobDefaultOption = cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow

type MaintenanceWindowResult struct {
	Allowed          bool
	NextWindowStart  *time.Time
	CurrentWindowEnd *time.Time
}

func CheckMaintenanceWindow(spec *v1alpha1.MaintenanceSpec, now time.Time) (*MaintenanceWindowResult, error) {
	if spec == nil || len(spec.Windows) == 0 {
		return &MaintenanceWindowResult{Allowed: true}, nil
	}

	var earliestNext *time.Time

	for _, window := range spec.Windows {
		result, err := checkWindow(window, now)
		if err != nil {
			return nil, err
		}
		if result.Allowed {
			return result, nil
		}
		if result.NextWindowStart != nil && (earliestNext == nil || result.NextWindowStart.Before(*earliestNext)) {
			earliestNext = result.NextWindowStart
		}
	}

	return &MaintenanceWindowResult{
		Allowed:         false,
		NextWindowStart: earliestNext,
	}, nil
}

func checkWindow(window v1alpha1.WindowSpec, now time.Time) (*MaintenanceWindowResult, error) {
	location, err := time.LoadLocation(window.Timezone)
	if err != nil {
		return nil, err
	}
	specParser := cron.NewParser(CronjobDefaultOption)
	sched, err := specParser.Parse(window.Start)
	if err != nil {
		return nil, err
	}

	localNow := now.In(location)
	lastFire := lastFireTime(sched, localNow)

	if !lastFire.IsZero() {
		windowEnd := lastFire.Add(window.Duration.Duration)
		if localNow.Before(windowEnd) {
			return &MaintenanceWindowResult{
				Allowed:          true,
				CurrentWindowEnd: &windowEnd,
			}, nil
		}
	}

	next := sched.Next(localNow)
	return &MaintenanceWindowResult{
		Allowed:         false,
		NextWindowStart: &next,
	}, nil
}

func lastFireTime(sched cron.Schedule, now time.Time) time.Time {
	cursor := sched.Next(now.Add(-7 * 24 * time.Hour))
	var lastFire time.Time
	for !cursor.After(now) {
		lastFire = cursor
		cursor = sched.Next(cursor)
	}
	return lastFire
}
