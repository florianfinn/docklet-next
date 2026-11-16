// Source: docklet-next/issues/466
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original non-credential workspace policy refusal.

package project

// WorkspaceProblem is a policy refusal, independently of agent authentication.
type WorkspaceProblem struct {
	Status int    `json:"status"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Code   string `json:"code" enum:"project.workspace-protected-path,project.workspace-path-refused"`
}

func (p *WorkspaceProblem) Error() string  { return p.Detail }
func (p *WorkspaceProblem) GetStatus() int { return p.Status }
