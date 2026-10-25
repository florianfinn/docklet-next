// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package image

import (
	"encoding/json"
	"errors"
	"github.com/distribution/reference"
	"strings"
)

// OperationError is the neutral contract shared by image and registry surfaces.
type OperationError struct {
	Status  int     `json:"-"`
	Success bool    `json:"success"`
	Problem Problem `json:"error"`
}
type Problem struct {
	Code    string `json:"code" enum:"image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.exists,registry.inline-credentials-not-allowed,registry.invalid-config,registry.not-found,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,update.agent-local-job-disabled,update.auto-excluded,update.invalid-policy,update.policy-changed,update.schedule-hub-owned,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-target,update.path-retired,update.persistence-failed,update.requires-hub-update,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found"`
	Message string `json:"message"`
}

func (e *OperationError) Error() string  { return e.Problem.Message }
func (e *OperationError) GetStatus() int { return e.Status }
func Failure(code string, status int) *OperationError {
	return &OperationError{Status: status, Problem: Problem{Code: code, Message: "The operation could not be completed."}}
}
func Neutral(err error, code string, status int) *OperationError {
	var problem *OperationError
	if errors.As(err, &problem) {
		return problem
	}
	return Failure(code, status)
}

type Protection struct {
	Reason  string `json:"reason"`
	OwnerID string `json:"ownerId,omitempty"`
}
type PruneItem struct {
	ID   string `json:"id"`
	Code string `json:"code" enum:"image.active-update-protected,image.busy,image.changed,image.in-use,image.invalid-archive,image.invalid-prune,image.invalid-reference,image.load-failed,image.not-found,image.prune-failed,image.pull-failed,image.remove-failed,image.reserved-reference,image.restore-point-protected,image.system-protected,image.tag-failed,image.usage-unavailable,registry.authentication-failed,registry.authentication-required,registry.credential-unavailable,registry.inline-credentials-not-allowed,registry.invalid-config,registry.persistence-failed,registry.rate-limited,registry.sync-failed,registry.unavailable,restore.deploy-failed,restore.image-mismatch,restore.in-use,restore.not-found,restore.shared-reference,restore.unavailable,update.agent-local-job-disabled,update.auto-excluded,update.invalid-policy,update.policy-changed,update.schedule-hub-owned,update.baseline-unavailable,update.check-unavailable,update.config-drift,update.deploy-failed,update.image-binding-unsupported,update.invalid-target,update.path-retired,update.persistence-failed,update.requires-hub-update,update.snapshot-unavailable,update.system-version-unknown,update.target-changed,update.target-not-found"`
}

func (p *PullOptions) UnmarshalJSON(data []byte) error {
	type plain PullOptions
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return Failure("image.invalid-reference", 400)
	}
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(data, &keys); err != nil {
		return Failure("image.invalid-reference", 400)
	}
	*p = PullOptions(decoded)
	for key := range keys {
		p.InlineCredentialsProvided = p.InlineCredentialsProvided || InlineCredentialKey(key)
	}
	return nil
}
func (p PullOptions) Validate() error {
	if p.InlineCredentialsProvided || p.Auth != nil || p.Credentials != nil {
		return Failure("registry.inline-credentials-not-allowed", 400)
	}
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(p.ImageName))
	if err != nil {
		return Failure("image.invalid-reference", 400)
	}
	if p.Tag != "" {
		if _, ok := named.(reference.Canonical); ok {
			return Failure("image.invalid-reference", 400)
		}
		if tagged, ok := named.(reference.Tagged); ok && tagged.Tag() != p.Tag {
			return Failure("image.invalid-reference", 400)
		}
	}
	return nil
}

// InlineCredentialKey recognises legacy registry authentication alternatives.
func InlineCredentialKey(key string) bool {
	key = strings.ToLower(strings.NewReplacer("-", "", "_", "").Replace(key))
	switch key {
	case "auth", "credentials", "registryauth", "registrycredentials", "xregistryauth", "xregistryconfig", "username", "password", "token":
		return true
	}
	return false
}
