// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original validation of bounded external history samples.

package history

import (
	dto "github.com/florianfinn/docklet-next/types/history"
	"math"
	"reflect"
	"slices"
	"time"
)

func Validate(s dto.Sample) error {
	invalid := func() error { return dto.Fail("history.sample-invalid", 502) }
	if len(s.Containers) > 2000 || len(s.Projects) > 1000 || !validNumbers(reflect.ValueOf(s)) {
		return invalid()
	}
	ids, names, projects := map[string]bool{}, map[string]bool{}, map[string]bool{}
	nameOK := func(v string) bool { return len(v) <= 255 }
	for _, c := range s.Containers {
		if c.ID == "" || c.Name == "" || !nameOK(c.ID) || !nameOK(c.Name) || !nameOK(c.ProjectName) || !nameOK(c.ProjectID) || !nameOK(c.Service) || ids[c.ID] || names[c.Name] || c.RestartCount < 0 || c.ExitCode < 0 {
			return invalid()
		}
		ids[c.ID], names[c.Name] = true, true
		if !slices.Contains([]string{"running", "restarting", "paused", "exited", "created", "dead", "removing"}, c.State) || !slices.Contains([]string{"", "none", "starting", "healthy", "unhealthy"}, c.Health) {
			return invalid()
		}
		for _, timestamp := range []string{c.StartedAt, c.FinishedAt} {
			if timestamp != "" {
				if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
					return invalid()
				}
			}
		}
		if c.State == "running" && c.Stats == nil && !c.StatsMissing {
			return invalid()
		}
	}
	if len(s.Uninspected) > 2000 {
		return invalid()
	}
	for _, id := range s.Uninspected {
		if id == "" || !nameOK(id) || ids[id] {
			return invalid()
		}
		ids[id] = true
	}
	for _, p := range s.Projects {
		if p.ID == "" || p.Name == "" || !nameOK(p.ID) || !nameOK(p.Name) || projects[p.ID] {
			return invalid()
		}
		projects[p.ID] = true
	}
	return nil
}
func validNumbers(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Float64:
		return !math.IsNaN(v.Float()) && !math.IsInf(v.Float(), 0) && v.Float() >= 0
	case reflect.Pointer:
		return v.IsNil() || validNumbers(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !validNumbers(v.Field(i)) {
				return false
			}
		}
	case reflect.Slice:
		for i := 0; i < v.Len(); i++ {
			if !validNumbers(v.Index(i)) {
				return false
			}
		}
	}
	return true
}
