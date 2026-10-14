// Source: florianfinn/docklet-next/backend/internal/activity
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: New run trace projection.

package activity

import (
	"strings"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
)

func servicesJSONInternal(services []string) database.JSON {
	if services == nil {
		return nil
	}
	clean := make([]string, 0, len(services))
	seen := make(map[string]bool, len(services))
	for _, service := range services {
		service = strings.TrimSpace(service)
		if service != "" && !seen[service] {
			clean = append(clean, service)
			seen[service] = true
		}
	}
	return database.JSON{"services": clean}
}

func servicesFromJSONInternal(value database.JSON) []string {
	if value == nil {
		return nil
	}
	var result []string
	switch services := value["services"].(type) {
	case []string:
		result = append(result, services...)
	case []any:
		for _, service := range services {
			if name, ok := service.(string); ok {
				result = append(result, name)
			}
		}
	}
	return result
}

func stepTraceInternal(messages []ActivityMessage, run *Activity) []activitytypes.StepTraceEntry {
	trace := make([]activitytypes.StepTraceEntry, 0)
	for _, message := range messages {
		name := strings.TrimSpace(message.Step)
		if name == "" {
			continue
		}
		if len(trace) > 0 {
			previous := &trace[len(trace)-1]
			if previous.Name == name {
				continue
			}
			previous.Status = activitytypes.StatusSuccess
			previous.EndedAt = new(message.CreatedAt)
		}
		trace = append(trace, activitytypes.StepTraceEntry{
			ID: message.ID, Name: name, Status: activitytypes.StatusRunning,
			Code: message.Code, StartedAt: message.CreatedAt,
		})
	}
	if len(trace) > 0 && run.EndedAt != nil {
		last := &trace[len(trace)-1]
		last.Status = run.Status
		last.EndedAt = run.EndedAt
	}
	return trace
}
