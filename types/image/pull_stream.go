// Source: docklet-next/docs/design/images-updates.md
// Revision: 7df52f8f3b912be751e2283ec8d990933f5dc4b8
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original image/registry UI foundation follow-up for issue #229.

package image

type PullStartedFrame struct {
	Type       string `json:"type" enum:"activity"`
	ActivityID string `json:"activityId" minLength:"1"`
}

type PullProgressFrame struct {
	Status         string              `json:"status" enum:"Pulling image,Pulling fs layer,Waiting,Downloading,Verifying Checksum,Download complete,Extracting,Pull complete,Already exists"`
	ID             string              `json:"id,omitempty" pattern:"^[a-f0-9]{12,64}$"`
	ProgressDetail *PullProgressCounts `json:"progressDetail,omitempty"`
}

type PullProgressCounts struct {
	Current int64 `json:"current" minimum:"0"`
	Total   int64 `json:"total" minimum:"0"`
}

type PullErrorFrame struct {
	Error Problem `json:"error"`
}

type PullDoneFrame struct {
	Done bool `json:"done" enum:"true"`
}
