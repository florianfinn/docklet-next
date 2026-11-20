// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

// Package proxy owns the shared journal and one-dispatch protocol. Engines own
// syntax, graph validation and verification; they cannot bypass phase commits.
package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"path"
	"regexp"
	"strings"
	"time"
)

type Bytes struct {
	dto.FileState
	Content []byte `json:"content"`
}

func Snapshot(b []byte, present bool) Bytes {
	v := Bytes{FileState: dto.FileState{Present: present}, Content: b}
	if present {
		h := sha256.Sum256(b)
		v.Hash = hex.EncodeToString(h[:])
	}
	return v
}

type Intent struct {
	Kind          dto.OperationKind `json:"kind,omitempty"`
	Request       dto.ApplyRequest  `json:"request"`
	Old           Bytes             `json:"old"`
	New           Bytes             `json:"new"`
	Mode          uint32            `json:"mode"`
	GraphHashes   map[string]string `json:"graphHashes"`
	TempPath      string            `json:"tempPath"`
	MarkerPath    string            `json:"markerPath"`
	StagingPath   string            `json:"stagingPath"`
	Reload        bool              `json:"reload"`
	Restart       bool              `json:"restart"`
	StartedBefore string            `json:"startedBefore"`
	Owned         bool              `json:"owned"`
	// PublishedAt bounds log evidence; it is persisted with the publishing commit.
	PublishedAt time.Time `json:"publishedAt"`
	// Engine is opaque engine state from Prepare (for example the pre-change probe),
	// passed back unchanged to every later engine call, also after a restart.
	Engine jsontext.Value `json:"engine,omitempty"`
}

// Runtime is the guarded host boundary. Publish rechecks the complete graph and
// target immediately before rename; Restore accepts only old/new, never third.
type Runtime interface {
	Check(context.Context, dto.Instance) error
	Target(context.Context, dto.Instance, string) (Bytes, error)
	Graph(context.Context, dto.Instance) (map[string]string, error)
	Stage(context.Context, Intent) error
	Publish(context.Context, Intent) error
	Restore(context.Context, Intent) error
	Cleanup(context.Context, Intent) error
	Epoch(context.Context, dto.Instance) (running bool, startedAt string, err error)
	// Rebind returns the instance bound to the current container when the
	// configuration source at the same mount target is unchanged.
	Rebind(context.Context, dto.Instance) (dto.Instance, error)
}
type Observation struct {
	Certificate *dto.CertificateObservation
	Probe       *dto.ProbeEvidence
	State       dto.State
	Reason      dto.Reason
	Evidence    dto.EvidenceKind
	NeedsMarker bool
	Restore     bool
}

// IncompleteRead marks a Read result shown as incomplete inventory that rests on
// a failed infrastructure read: displays use the result, publication checks
// keep Err (a transient error, see Transient).
type IncompleteRead struct{ Err error }

func (e *IncompleteRead) Error() string { return e.Err.Error() }
func (e *IncompleteRead) Unwrap() error { return e.Err }

// Engines receive immutable intent values. Validate may use StagingPath; Cleanup
// must remove engine staging, and Runtime removes temporary and marker files.
type Engine interface {
	Inventory(context.Context, dto.Instance) (dto.Inventory, error)
	Read(context.Context, dto.Instance) (dto.ReadResult, error)
	Prepare(context.Context, dto.ApplyRequest) (Intent, error)
	Validate(context.Context, Intent) error
	Reload(context.Context, Intent) error
	Restart(context.Context, Intent) error
	Observe(context.Context, Intent, bool) (Observation, error)
	Mark(context.Context, Intent) error
	Cleanup(context.Context, Intent) error
}
type Engines map[dto.Kind]Engine

func (e Engines) For(k dto.Kind) (Engine, error) {
	v := e[k]
	if v == nil {
		return nil, dto.Fail(dto.EngineUnavailable, 503)
	}
	return v, nil
}
func Digest(r dto.ApplyRequest) (string, error) {
	// Explicit route and the legacy absent discriminator identify the same request.
	if r.Kind.Effective() == dto.RouteOperation {
		r.Kind = ""
	}
	b, err := json.Marshal(r, json.Deterministic(true))
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

var safeID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$`)

func validRequest(r dto.ApplyRequest) error {
	if r.Kind.Effective() != dto.RouteOperation {
		return dto.Fail(dto.UnknownOperationKind, 409)
	}
	if !safeID.MatchString(r.ID) || r.Instance.ID == "" || r.Generation == "" || r.Registration == "" || r.Instance.Source.Canonical == "" {
		return dto.Fail(dto.InvalidInput, 422)
	}
	if r.Target == "" || r.Target == "." || path.IsAbs(r.Target) || path.Clean(r.Target) != r.Target || r.Target == ".." || strings.HasPrefix(r.Target, "../") || strings.ContainsAny(r.Target, "\x00\r\n") {
		return dto.Fail(dto.InvalidInput, 422)
	}
	for _, s := range strings.Split(r.Target, "/") {
		if strings.HasPrefix(s, ".docklet-") || strings.HasPrefix(s, "docklet-check-") {
			return dto.Fail(dto.InvalidInput, 422)
		}
		if strings.Contains(strings.ToLower(s), ".secret") {
			return dto.Fail(dto.OpaqueInput, 422)
		}
	}
	return nil
}
func paths(i *Intent) {
	d := path.Dir(i.Request.Target)
	i.TempPath = path.Join(d, ".docklet-"+i.Request.ID+".tmp")
	i.StagingPath = path.Join(d, ".docklet-staging-"+i.Request.ID)
	if i.Request.Instance.Kind == dto.Traefik {
		i.MarkerPath = path.Join(d, "docklet-check-"+i.Request.ID+".yml")
	}
}
func Eligibility(i dto.Instance) error {
	if !i.Eligibility.Writable || i.Source.ReadOnly || i.Source.Kind == dto.SingleFileSource || i.Source.Kind == dto.UnknownSourceKind {
		p := dto.Fail(dto.SourceReadOnly, 409)
		// The detail names the eligibility reason and, when inventory kept it, the
		// engine's own bounded text (for example a failed validation).
		p.Detail = string(i.Eligibility.Reason)
		if i.Eligibility.Detail != "" {
			p.Detail += ": " + i.Eligibility.Detail
		}
		p.Path, p.Line = i.Eligibility.Path, i.Eligibility.Line
		if d := diagnostic(p); d != nil {
			p.Path, p.Line, p.Detail = d.Path, d.Line, d.Detail
		}
		return p
	}
	return nil
}
func equalGraph(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
func stale(p string) error { return &dto.Problem{Status: 409, Code: dto.StaleHash, Path: p} }
