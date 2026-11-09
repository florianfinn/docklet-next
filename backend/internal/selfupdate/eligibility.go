// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	"fmt"
	"strings"

	"github.com/distribution/reference"
	dto "github.com/florianfinn/docklet-next/types/system"
	product "github.com/florianfinn/docklet-next/types/version"
)

const (
	CodeNotCompose = "update.self-not-compose"
	CodeLayout     = "update.self-unsupported-layout"
	CodeCustom     = "update.self-custom-image"
	CodeHub        = "update.requires-hub-update"
	CodeFailed     = "update.self-failed"
	CodeUnknown    = "update.outcome-unknown"
)

type Problem struct {
	Code   string
	Status int
}

func (e *Problem) Error() string  { return e.Code }
func (e *Problem) GetStatus() int { return e.Status }
func Refuse(code string) error    { return &Problem{Code: code, Status: 409} }

// Installation describes one inspected container. RepoDigests are deliberately
// absent: Config.Image and the running binary are the sources of identity.
type Installation struct {
	Role, Version, CurrentReference, Repository string
	Compose, Replayable, Busy                   bool
}

type Feed struct {
	Candidate *dto.SelfUpdateCandidate
	Code      string
	Checking  bool
}

// Eligibility constructs an independent value for every host, including all
// pointer/slice fields. The caller may share a cached Feed across requests.
func Eligibility(host Installation, feed Feed, hubVersion string) dto.SelfUpdateEligibility {
	out := dto.SelfUpdateEligibility{State: "current", Manual: []string{"docker compose pull", "docker compose up -d --no-deps <service>"}}
	if feed.Candidate != nil {
		c := *feed.Candidate
		out.Candidate = &c
	}
	refuse := func(state, code string) dto.SelfUpdateEligibility { out.State = state; out.Code = code; return out }
	if !host.Compose {
		return refuse("not-eligible", CodeNotCompose)
	}
	if !OfficialReference(host.CurrentReference, host.Repository, host.Version) {
		return refuse("not-eligible", CodeCustom)
	}
	if !host.Replayable {
		return refuse("not-eligible", CodeLayout)
	}
	if host.Busy {
		return refuse("not-eligible", "host.busy")
	}
	if feed.Code != "" {
		return refuse("unavailable", feed.Code)
	}
	if feed.Checking {
		return refuse("checking", "")
	}
	current, ok := product.ParseProduct(host.Version)
	if !ok {
		return refuse("unavailable", "update.system-version-unknown")
	}
	if out.Candidate == nil {
		return out
	}
	target, valid := product.ParseProduct(out.Candidate.Version)
	if !valid || !target.Stable() {
		return refuse("unavailable", "update.check-unavailable")
	}
	if target.Compare(current) <= 0 {
		out.Candidate = nil
		return out
	}
	if host.Role == "agent" && hubVersion != "" {
		hub, valid := product.ParseProduct(hubVersion)
		if !valid {
			return refuse("unavailable", "update.system-version-unknown")
		}
		if target.Compare(hub) > 0 {
			return refuse("not-eligible", CodeHub)
		}
	}
	out.State = "available"
	return out
}

func OfficialReference(raw, repository, version string) bool {
	named, err := reference.ParseNormalizedNamed(raw)
	if err != nil || named.Name() != repository {
		return false
	}
	tag, ok := named.(reference.Tagged)
	if !ok || tag.Tag() != version {
		return false
	}
	parsed, ok := product.ParseProduct(version)
	return ok && parsed.Stable() && parsed.String() == version
}

// ValidateEnvelope is evaluated on the actual agent before any pull or helper.
func ValidateEnvelope(request dto.SelfUpdateEnvelope, ownVersion, repository string) error {
	if !ValidOpID(request.OpID) {
		return Refuse("update.invalid-target")
	}
	target, ok := product.ParseProduct(request.TargetVersion)
	own, ownOK := product.ParseProduct(ownVersion)
	hub, hubOK := product.ParseProduct(request.HubVersion)
	if !ok || !ownOK || !hubOK || !target.Stable() || target.String() != request.TargetVersion || target.Compare(own) <= 0 {
		return Refuse("update.invalid-target")
	}
	if target.Compare(hub) > 0 {
		return Refuse(CodeHub)
	}
	if !OfficialReference(request.TargetReference, repository, request.TargetVersion) {
		return Refuse("update.invalid-target")
	}
	named, _ := reference.ParseNormalizedNamed(request.TargetReference)
	digest, ok := named.(reference.Canonical)
	if !ok || digest.Digest().Algorithm() != "sha256" || digest.Digest().Validate() != nil || !strings.HasSuffix(request.TargetReference, "@"+digest.Digest().String()) {
		return Refuse("update.invalid-target")
	}
	return nil
}

func ValidOpID(id string) bool {
	if len(id) < 1 || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}
func HelperName(id string) string { return fmt.Sprintf("docklet-self-update-%s", id) }
