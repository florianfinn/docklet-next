// Source: getarcaneapp/arcane/types/backup/durable_run.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package backup

import "github.com/florianfinn/docklet-next/types/scheduler"

// DurableRunCommand freezes the input accepted by a backup domain.
type DurableRunCommand struct {
	UserID           string `msgpack:"userID"`
	EnvironmentID    string `msgpack:"environmentID"`
	Permission       string `msgpack:"permission"`
	RequestedWithKey string `msgpack:"requestedWithKey"`
	Kind             string `msgpack:"kind"`
	RunID            string `msgpack:"runID"`
	ActivityID       string `msgpack:"activityID"`
	Payload          []byte `msgpack:"payload"`
}

// DurableRunState records dispatch and effect evidence across host restarts.
type DurableRunState struct {
	Command DurableRunCommand         `msgpack:"command"`
	Started bool                      `msgpack:"started"`
	Status  scheduler.RunStatus       `msgpack:"status"`
	Targets []scheduler.TargetOutcome `msgpack:"targets"`
	Error   string                    `msgpack:"error"`
}
