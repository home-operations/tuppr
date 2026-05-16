package maintenance

import (
	"time"

	"github.com/home-operations/tuppr/api/v1alpha1"
	"github.com/netresearch/go-cron"
)

var CronjobDefaultOption = cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow

const RequeueCap = 5 * time.Minute

type WindowResult struct {
	Allowed          bool
	NextWindowStart  *time.Time
	CurrentWindowEnd *time.Time
}

// RequeueAfter returns the delay until NextWindowStart, capped at RequeueCap.
// Returns 0 when the window is currently open or no next window is known.
func (r *WindowResult) RequeueAfter(now time.Time) time.Duration {
	if r.Allowed || r.NextWindowStart == nil {
		return 0
	}
	d := r.NextWindowStart.Sub(now)
	if d > RequeueCap {
		return RequeueCap
	}
	return d
}

func CheckWindow(spec *v1alpha1.MaintenanceSpec, now time.Time) (*WindowResult, error) {
	if spec == nil || len(spec.Windows) == 0 {
		return &WindowResult{Allowed: true}, nil
	}

	var earliestNext *time.Time

	for _, window := range spec.Windows {
		result, err := checkSingleWindow(window, now)
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

	return &WindowResult{
		Allowed:         false,
		NextWindowStart: earliestNext,
	}, nil
}

func checkSingleWindow(window v1alpha1.WindowSpec, now time.Time) (*WindowResult, error) {
	location, err := time.LoadLocation(window.Timezone)
	if err != nil {
		return nil, err
	}
	specParser := cron.MustNewParser(CronjobDefaultOption)
	sched, err := specParser.Parse(window.Start)
	if err != nil {
		return nil, err
	}

	localNow := now.In(location)
	lastFire := lastFireTime(sched, localNow)

	if !lastFire.IsZero() {
		windowEnd := lastFire.Add(window.Duration.Duration)
		if localNow.Before(windowEnd) {
			return &WindowResult{
				Allowed:          true,
				CurrentWindowEnd: &windowEnd,
			}, nil
		}
	}

	next := sched.Next(localNow)
	return &WindowResult{
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
