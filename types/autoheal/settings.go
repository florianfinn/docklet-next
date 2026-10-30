// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package autoheal

import "regexp"

type Settings struct {
	Enabled              bool     `json:"enabled"`
	MaxRestarts          int      `json:"maxRestarts" minimum:"1" maximum:"10"`
	WindowMinutes        int      `json:"windowMinutes" minimum:"5" maximum:"1440"`
	ExcludedStacks       []string `json:"excludedStacks" maxItems:"200"`
	ExcludedContainers   []string `json:"excludedContainers" maxItems:"200"`
	ManagedByEnvironment bool     `json:"managedByEnvironment" readOnly:"true"`
}
type Update struct {
	Enabled            bool     `json:"enabled"`
	MaxRestarts        int      `json:"maxRestarts"`
	WindowMinutes      int      `json:"windowMinutes"`
	ExcludedStacks     []string `json:"excludedStacks"`
	ExcludedContainers []string `json:"excludedContainers"`
}
type Problem struct {
	Code   string `json:"code" enum:"autoheal.settings-invalid,autoheal.settings-env-managed,settings.key-managed"`
	Field  string `json:"field,omitempty"`
	Status int    `json:"status"`
}

func (p *Problem) Error() string { return p.Code }

var stackName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
var containerName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

func (u Update) Validate() error {
	invalid := func(f string) error { return &Problem{Code: "autoheal.settings-invalid", Field: f, Status: 400} }
	if u.MaxRestarts < 1 || u.MaxRestarts > 10 {
		return invalid("/maxRestarts")
	}
	if u.WindowMinutes < 5 || u.WindowMinutes > 1440 {
		return invalid("/windowMinutes")
	}
	if len(u.ExcludedStacks) > 200 {
		return invalid("/excludedStacks")
	}
	if len(u.ExcludedContainers) > 200 {
		return invalid("/excludedContainers")
	}
	for _, n := range u.ExcludedStacks {
		if len(n) > 63 || !stackName.MatchString(n) {
			return invalid("/excludedStacks")
		}
	}
	for _, n := range u.ExcludedContainers {
		if len(n) > 128 || !containerName.MatchString(n) {
			return invalid("/excludedContainers")
		}
	}
	return nil
}

func (p *Problem) GetStatus() int { return p.Status }
