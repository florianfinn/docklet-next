// Source: getarcaneapp/arcane/types/jobschedule/runs.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package jobschedule

import st "github.com/florianfinn/docklet-next/types/scheduler"

type JobInput struct {
	ID    string `path:"id"`
	JobID string `path:"jobId" minLength:"1"`
}
type ListRunsInput struct {
	ID    string `path:"id"`
	JobID string `path:"jobId" minLength:"1"`
	Page  int    `query:"page" default:"1" minimum:"1"`
	Limit int    `query:"limit" default:"20" minimum:"1" maximum:"100"`
}
type RunInput struct {
	ID    string `path:"id"`
	JobID string `path:"jobId" minLength:"1"`
	RunID string `path:"runId" format:"uuid"`
}
type (
	RunOutput      struct{ Body st.Run }
	ListRunsOutput struct{ Body st.RunList }
)

// ResolveRunInput accepts the original operator identity only from trusted agent transport.
// TODO(v3): remove this deprecated mixed-version compatibility contract.
type ResolveRunInput struct {
	RunInput

	Body *struct {
		ResolvedBy string `json:"resolvedBy,omitempty"`
	}
}
