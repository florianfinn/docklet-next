// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original optional-module management contract.

package modules

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/features"
)

type State struct {
	ID      features.ID   `json:"id"`
	Enabled bool          `json:"enabled"`
	Kind    features.Kind `json:"kind" enum:"module"`
}

type PendingCleanup struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type DeleteResult struct {
	PendingCleanup []PendingCleanup `json:"pendingCleanup" nullable:"false"`
}

// ModuleProblem describes module admission and lifecycle refusals without exposing host files.
type ModuleProblem struct {
	Status    int         `json:"status"`
	Code      string      `json:"code" enum:"module-busy,module-on,feature-disabled,operation-conflict,agent.update-required"`
	Feature   features.ID `json:"feature"`
	Operation string      `json:"operation,omitempty"`
}

func (p *ModuleProblem) Error() string             { return p.Code }
func (p *ModuleProblem) GetStatus() int            { return p.Status }
func (p *ModuleProblem) ContentType(string) string { return "application/problem+json" }

// Problem is the shared module problem contract.
type Problem = ModuleProblem

// DisabledRefusal distinguishes a synchronised module veto from rejected agent
// credentials. Only a bounded, matching typed 403 may cross the hub unchanged.
func DisabledRefusal(status int, decodedPath string, body []byte) bool {
	if status != 403 || len(body) > 16<<10 {
		return false
	}
	definition, ok := features.ForPath(decodedPath)
	if !ok {
		return false
	}
	var p ModuleProblem
	return json.Unmarshal(body, &p) == nil && p.Code == "feature-disabled" && p.Status == 403 && p.Feature == definition.ID
}
