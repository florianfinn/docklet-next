// Source: getarcaneapp/arcane/types/updater/durable.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package updater

// SingleUpdateCommand contains the persisted inputs for an asynchronous update.
type SingleUpdateCommand struct {
	ContainerID string
	ActivityID  string
	UserID      string
	KeyID       string
}

type SingleUpdateState struct {
	Command SingleUpdateCommand
	Status  string
	Target  *FrozenUpdateTarget
	Result  *Result
	Failure string
}

// FrozenUpdateTarget identifies the selected container and immutable image intent.
type FrozenUpdateTarget struct {
	ContainerID          string
	ContainerName        string
	ComposeProject       string
	ComposeService       string
	ComposeNumber        string
	BaselineImageID      string
	BaselineStartedAt    string
	BaselineRestartCount int
	DesiredImageRef      string
	DesiredDigest        string
}
