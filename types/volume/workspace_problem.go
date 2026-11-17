// Source: docklet-next/issues/507
// Revision: 8380898628dbb860f193773e2e3822dacb783269
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original non-credential volume workspace policy refusal.

package volume

// WorkspaceProblem is a path policy refusal, independently of agent authentication.
type WorkspaceProblem struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code" enum:"volume.workspace-path-refused"`
}

func (p *WorkspaceProblem) Error() string  { return p.Detail }
func (p *WorkspaceProblem) GetStatus() int { return p.Status }
func (p *WorkspaceProblem) ContentType(contentType string) string {
	return (&HelperIncompatibleError{}).ContentType(contentType)
}
