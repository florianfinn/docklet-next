// Source: getarcaneapp/arcane/backend/pkg/scheduler/schedule/cron.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: One location-aware parse entry point for validation, execution and projection; M18 live scheduling, policy and contract foundation.

// Package schedule provides reusable schedule validation.
package schedule

import (
	"fmt"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
)

// NormalizeSixField validates and normalizes a six-field cron schedule.
func NormalizeSixField(value, subject string) (string, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "", fmt.Errorf("%s schedule is required", subject)
	}
	if len(fields) != 6 {
		return "", fmt.Errorf("invalid %s schedule %q: expected six fields", subject, strings.TrimSpace(value))
	}
	normalized := strings.Join(fields, " ")
	if _, err := Parse(normalized, time.UTC); err != nil {
		return "", fmt.Errorf("invalid %s schedule %q: %w", subject, normalized, err)
	}
	return normalized, nil
}

func parser() cron.Parser {
	return cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
}

// Parse applies the shared wall-clock rule in loc. All-hours specifications and
// constant delays retain cron's elapsed-time behavior, including repeated hours.
func Parse(expr string, loc *time.Location) (cron.Schedule, error) {
	if loc == nil {
		loc = time.UTC
	}
	parsed, err := parser().Parse(expr)
	if err != nil {
		return nil, err
	}
	if spec, ok := parsed.(*cron.SpecSchedule); ok {
		spec.Location = loc
		const allHours uint64 = (1 << 24) - 1
		if spec.Hour&allHours != allHours {
			floating := *spec
			floating.Location = time.UTC
			return wallSchedule{spec: &floating, location: loc}, nil
		}
	}
	return parsed, nil
}

type wallSchedule struct {
	spec     *cron.SpecSchedule
	location *time.Location
}

func wall(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
}

func (s wallSchedule) Next(t time.Time) time.Time {
	cursor := wall(t.In(s.location)).Add(-3 * time.Hour)
	var best, until time.Time
	for {
		w := s.spec.Next(cursor)
		if w.IsZero() || (!until.IsZero() && w.After(until)) {
			return best
		}
		instant := s.instant(w)
		if instant.After(t) {
			if best.IsZero() || instant.Before(best) {
				best = instant
			}
			if until.IsZero() {
				until = w.Add(3 * time.Hour)
			}
		}
		cursor = w
	}
}

// A fold chooses the earlier matching instant. In a gap, interpreting W with
// the preceding offset moves it forward by the transition's actual gap.
func (s wallSchedule) instant(w time.Time) time.Time {
	offsets := map[int]bool{}
	for delta := -36 * time.Hour; delta <= 36*time.Hour; delta += 6 * time.Hour {
		_, offset := w.Add(delta).In(s.location).Zone()
		offsets[offset] = true
	}
	var exact, shifted, shiftedWall time.Time
	for offset := range offsets {
		candidate := w.Add(-time.Duration(offset) * time.Second)
		localWall := wall(candidate.In(s.location))
		if localWall.Equal(w) && (exact.IsZero() || candidate.Before(exact)) {
			exact = candidate
		}
		if localWall.After(w) && (shifted.IsZero() || localWall.Before(shiftedWall)) {
			shifted, shiftedWall = candidate, localWall
		}
	}
	if !exact.IsZero() {
		return exact
	}
	return shifted
}
