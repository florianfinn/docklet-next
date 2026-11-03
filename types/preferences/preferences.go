// Source: docklet-next/issues/339
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original personal preference wire document.

package preferences

const SectionLimit = 16 * 1024
const DocumentLimit = 64 * 1024

type Document struct {
	Revision int64                     `json:"revision"`
	Sections map[string]map[string]any `json:"sections"`
}

type Update struct {
	Value any `json:"value"`
}

type Problem struct {
	Code   string `json:"code" enum:"preferences.no-user,preferences.unknown-section,preferences.invalid,preferences.too-large"`
	Status int    `json:"status"`
}

func (p *Problem) Error() string  { return p.Code }
func (p *Problem) GetStatus() int { return p.Status }

func KnownSection(section string) bool {
	switch section {
	case "appearance", "uiStyle", "visualStyle", "views", "hidden", "help", "dashboard", "fleetView", "favorite", "sidebar":
		return true
	}
	return false
}
