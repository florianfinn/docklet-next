// Source: getarcaneapp/arcane/backend/internal/system/children/prune/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; preserve scheduled network-prune removal semantics; M16 bind mounts and backup completion; M18 live scheduling, policy and contract foundation.

package prune

import (
	"fmt"
	"strconv"
	"time"
)

func (s *Service) pruneCutoffInternal(raw string) (time.Time, error) {
	now := time.Now()
	location := s.location
	if s.zones != nil {
		location = s.zones.Location()
	}
	if location == nil {
		location = time.UTC
	}
	if duration, err := time.ParseDuration(raw); err == nil && duration >= 0 {
		return now.Add(-duration), nil
	}
	for _, format := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02"} {
		if value, err := time.ParseInLocation(format, raw, location); err == nil {
			return value, nil
		}
	}
	if seconds, err := strconv.ParseFloat(raw, 64); err == nil {
		return time.Unix(int64(seconds), int64((seconds-float64(int64(seconds)))*1e9)), nil
	}
	return time.Time{}, fmt.Errorf("unsupported prune cutoff %q", raw)
}
