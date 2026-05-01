package crontabthread

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/teambition/rrule-go"
)

func loadLocation(tz string) (*time.Location, string, error) {
	tz = strings.TrimSpace(tz)
	if tz == "" || strings.EqualFold(tz, "local") {
		return time.Local, "Local", nil
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, "", fmt.Errorf("invalid timezone %q: %w", tz, err)
	}
	return loc, tz, nil
}

func computeNextRun(now time.Time, scheduleType string, runAt time.Time, cronExpr string, rruleText string, loc *time.Location) (time.Time, error) {
	rruleText = strings.TrimSpace(rruleText)
	if rruleText != "" {
		opt, err := rrule.StrToROption(rruleText)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid rrule: %w", err)
		}
		opt.Dtstart = now.In(loc)
		r, err := rrule.NewRRule(*opt)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid rrule options: %w", err)
		}
		next := r.After(now.In(loc), false)
		if next.IsZero() {
			return time.Time{}, fmt.Errorf("rrule did not yield a next run time")
		}
		return next.UTC(), nil
	}

	switch strings.TrimSpace(scheduleType) {
	case "once":
		if runAt.IsZero() {
			return time.Time{}, fmt.Errorf("run_at is required for schedule_type=once")
		}
		runLocal := runAt.In(loc)
		if !runLocal.After(now.In(loc)) {
			return time.Time{}, fmt.Errorf("run_at must be in the future")
		}
		return runLocal.UTC(), nil
	case "recurring":
		cronExpr = strings.TrimSpace(cronExpr)
		if cronExpr == "" {
			return time.Time{}, fmt.Errorf("cron_expr or rrule is required for schedule_type=recurring")
		}
		parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		sched, err := parser.Parse(cronExpr)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid cron_expr: %w", err)
		}
		next := sched.Next(now.In(loc))
		if next.IsZero() {
			return time.Time{}, fmt.Errorf("cron_expr did not yield a next run time")
		}
		return next.UTC(), nil
	default:
		return time.Time{}, fmt.Errorf("unknown schedule_type %q", scheduleType)
	}
}
