// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation; M19 backend contracts and target identity integration.

package autoheal

import "regexp"

// Settings exposes effective values, including invalid environment values named by InvalidFields.
type Settings struct {
	Enabled              bool     `json:"enabled"`
	MaxRestarts          int      `json:"maxRestarts"`
	CooldownMinutes      int      `json:"cooldownMinutes"`
	InvalidFields        []string `json:"invalidFields" nullable:"false" enum:"maxRestarts,windowMinutes,cooldownMinutes,excludedStacks,excludedContainers" readOnly:"true"`
	WindowMinutes        int      `json:"windowMinutes"`
	ExcludedStacks       []string `json:"excludedStacks"`
	ExcludedContainers   []string `json:"excludedContainers"`
	ManagedByEnvironment bool     `json:"managedByEnvironment" readOnly:"true"`
}
type Update struct {
	Enabled            bool     `json:"enabled"`
	MaxRestarts        int      `json:"maxRestarts"`
	CooldownMinutes    *int     `json:"cooldownMinutes,omitempty"`
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

func (u Update) InvalidFieldNames() []string {
	fields := []string{}
	if u.MaxRestarts < 1 || u.MaxRestarts > 10 {
		fields = append(fields, "maxRestarts")
	}
	if u.WindowMinutes < 5 || u.WindowMinutes > 1440 {
		fields = append(fields, "windowMinutes")
	}
	if u.CooldownMinutes != nil && (*u.CooldownMinutes < 1 || *u.CooldownMinutes > 60) {
		fields = append(fields, "cooldownMinutes")
	}
	valid := func(values []string, maximum int, pattern *regexp.Regexp) bool {
		if len(values) > 200 {
			return false
		}
		for _, value := range values {
			if len(value) > maximum || !pattern.MatchString(value) {
				return false
			}
		}
		return true
	}
	if !valid(u.ExcludedStacks, 63, stackName) {
		fields = append(fields, "excludedStacks")
	}
	if !valid(u.ExcludedContainers, 128, containerName) {
		fields = append(fields, "excludedContainers")
	}
	return fields
}
func (s Settings) InvalidFieldNames() []string {
	return (Update{MaxRestarts: s.MaxRestarts, WindowMinutes: s.WindowMinutes, CooldownMinutes: &s.CooldownMinutes, ExcludedStacks: s.ExcludedStacks, ExcludedContainers: s.ExcludedContainers}).InvalidFieldNames()
}
func (u Update) Validate() error {
	if fields := u.InvalidFieldNames(); len(fields) > 0 {
		return &Problem{Code: "autoheal.settings-invalid", Field: "/" + fields[0], Status: 400}
	}
	return nil
}
func (p *Problem) GetStatus() int { return p.Status }
