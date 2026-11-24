// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 contract foundation; no proxy effects.

package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"time"
)

type OperationKind string

const (
	RouteOperation    OperationKind = "route"
	ResolverOperation OperationKind = "resolver"
	// AdoptionOperation records that an instance followed its recreated container (#592). It is written finished on the hub
	// and never dispatched to an agent.
	AdoptionOperation OperationKind = "adoption"
)

// An absent discriminator is the v19 route envelope.
func (k OperationKind) Effective() OperationKind {
	if k == "" {
		return RouteOperation
	}
	return k
}

type ResolverCA struct {
	Kind string `json:"kind" enum:"letsencrypt,letsencrypt-staging,custom"`
	URL  string `json:"url,omitempty" maxLength:"512"`
}
type ResolverChallenge struct {
	Kind             string      `json:"kind" enum:"http,dns"`
	EntryPoint       string      `json:"entryPoint,omitempty" maxLength:"128"`
	Provider         DNSProvider `json:"provider,omitempty" enum:"cloudflare,hetzner,ionos,inwx,netcup,desec,duckdns,httpreq"`
	DelayBeforeCheck int         `json:"delayBeforeCheck" minimum:"0" maximum:"600"`
	Resolvers        []string    `json:"resolvers,omitempty" maxItems:"4"`
}
type ResolverConfig struct {
	Name      string            `json:"name" pattern:"^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$"`
	Email     string            `json:"email" maxLength:"254"`
	CA        ResolverCA        `json:"ca"`
	Storage   string            `json:"storage" maxLength:"256"`
	Challenge ResolverChallenge `json:"challenge"`
}
type ResolverVariable struct {
	Name  string  `json:"name" pattern:"^[A-Z][A-Z0-9_]{0,63}$"`
	Value *string `json:"value,omitempty" minLength:"1" maxLength:"4096"`
}

// ResolverRequest is live input only. Persist Redact(), never this value.
type ResolverRequest struct {
	ResolverConfig
	Variables []ResolverVariable `json:"variables" maxItems:"16"`
}
type ResolverVariableIntent struct {
	Name string `json:"name"`
	Mode string `json:"mode" enum:"new,reuse"`
}
type ResolverIntent struct {
	ResolverConfig
	Variables []ResolverVariableIntent `json:"variables"`
}

func (q ResolverRequest) Redact() ResolverIntent {
	i := ResolverIntent{ResolverConfig: q.ResolverConfig, Variables: make([]ResolverVariableIntent, 0, len(q.Variables))}
	for _, v := range q.Variables {
		mode := "reuse"
		if v.Value != nil {
			mode = "new"
		}
		i.Variables = append(i.Variables, ResolverVariableIntent{Name: v.Name, Mode: mode})
	}
	return i
}
func (i ResolverIntent) Digest() (string, error) {
	b, err := json.Marshal(i)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

type ResolverApplyRequest struct {
	ID           string          `json:"id" minLength:"1" maxLength:"128"`
	Instance     Instance        `json:"instance"`
	Generation   string          `json:"generation"`
	Registration string          `json:"registration"`
	Request      ResolverRequest `json:"request"`
}

// ResolverEnvelope carries only redacted intent and binding across restarts.
type ResolverEnvelope struct {
	Kind         OperationKind  `json:"kind" enum:"resolver"`
	ID           string         `json:"id"`
	Instance     Instance       `json:"instance"`
	Generation   string         `json:"generation"`
	Registration string         `json:"registration"`
	Intent       ResolverIntent `json:"intent"`
}

// Digest binds the redacted intent to its operation, like the route digest:
// a replay must match ID, instance, generation and registration as well.
func (e ResolverEnvelope) Digest() (string, error) {
	b, err := json.Marshal(struct {
		Kind         OperationKind  `json:"kind"`
		ID           string         `json:"id"`
		InstanceID   string         `json:"instanceId"`
		Generation   string         `json:"generation"`
		Registration string         `json:"registration"`
		Intent       ResolverIntent `json:"intent"`
	}{e.Kind, e.ID, e.Instance.ID, e.Generation, e.Registration, e.Intent})
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

type ResolverSummary struct {
	Name      string                    `json:"name"`
	Email     string                    `json:"email,omitempty"`
	CA        ResolverCA                `json:"ca"`
	Storage   string                    `json:"storage"`
	Challenge ResolverChallenge         `json:"challenge"`
	Variables []ResolverVariableSummary `json:"variables"`
}
type ResolverVariableSummary struct {
	Name       string `json:"name"`
	Configured bool   `json:"configured"`
}
type ResolverPhase string

const (
	ResolverAdmitted         ResolverPhase = "admitted"
	ResolverBackup           ResolverPhase = "backup"
	ResolverWriting          ResolverPhase = "writing"
	ResolverRecreating       ResolverPhase = "recreating"
	ResolverObserving        ResolverPhase = "observing"
	ResolverApplied          ResolverPhase = "applied"
	ResolverRolledBack       ResolverPhase = "rolled-back"
	ResolverRefused          ResolverPhase = "refused"
	ResolverRestoring        ResolverPhase = "restoring"
	ResolverRecoveryRequired ResolverPhase = "recovery-required"
)

func (p ResolverPhase) Valid() bool {
	switch p {
	case ResolverAdmitted, ResolverBackup, ResolverWriting, ResolverRecreating, ResolverObserving, ResolverApplied, ResolverRolledBack, ResolverRefused, ResolverRestoring, ResolverRecoveryRequired:
		return true
	}
	return false
}
func (p ResolverPhase) Final() bool {
	return p == ResolverApplied || p == ResolverRolledBack || p == ResolverRefused
}

type ResolverFile struct {
	Path string    `json:"path"`
	Kind string    `json:"kind" enum:"static,compose,env"`
	Old  FileState `json:"old"`
	New  FileState `json:"new"`
}
type ResolverReplacement struct {
	ContainerID string     `json:"containerId"`
	Name        string     `json:"name"`
	StartedAt   string     `json:"startedAt"`
	Project     string     `json:"project"`
	Service     string     `json:"service"`
	MountTarget string     `json:"mountTarget"`
	Canonical   string     `json:"canonical"`
	Kind        SourceKind `json:"kind" enum:"project,volume,bind,single-file,unknown"`
}
type ResolverDiagnostic struct {
	Code     Reason `json:"code" enum:"compose-load-failed,compose-up-failed,static-parse-failed"`
	ExitCode *int   `json:"exitCode,omitempty"`
	Service  string `json:"service,omitempty"`
	Line     int    `json:"line,omitempty" minimum:"0"`
}
type ResolverReceipt struct {
	Intent            ResolverIntent          `json:"intent"`
	Phase             ResolverPhase           `json:"phase" enum:"admitted,backup,writing,recreating,observing,applied,rolled-back,refused,restoring,recovery-required"`
	Files             []ResolverFile          `json:"files"`
	Replacement       *ResolverReplacement    `json:"replacement,omitempty"`
	Diagnostic        *ResolverDiagnostic     `json:"diagnostic,omitempty"`
	LayoutReason      UnsupportedLayoutReason `json:"layoutReason,omitempty" enum:"unmanaged-project,multiple-replicas,multiple-compose-files,compose-include,service-extends,compose-anchor,env-not-regular,compose-env-selection,namespace-dependent,toml-static,entrypoint-flags,scalar-command,environment-static,mixed-static-sources,static-outside-project"`
	RecreateCommitted bool                    `json:"recreateCommitted"`
	RecreateStartedAt *time.Time              `json:"recreateStartedAt,omitempty"`
}
type UnsupportedLayoutReason string

const (
	UnmanagedProject     UnsupportedLayoutReason = "unmanaged-project"
	MultipleReplicas     UnsupportedLayoutReason = "multiple-replicas"
	MultipleComposeFiles UnsupportedLayoutReason = "multiple-compose-files"
	ComposeInclude       UnsupportedLayoutReason = "compose-include"
	ServiceExtends       UnsupportedLayoutReason = "service-extends"
	ComposeAnchor        UnsupportedLayoutReason = "compose-anchor"
	EnvNotRegular        UnsupportedLayoutReason = "env-not-regular"
	ComposeEnvSelection  UnsupportedLayoutReason = "compose-env-selection"
	NamespaceDependent   UnsupportedLayoutReason = "namespace-dependent"
	TOMLStatic           UnsupportedLayoutReason = "toml-static"
	EntrypointFlags      UnsupportedLayoutReason = "entrypoint-flags"
	ScalarCommand        UnsupportedLayoutReason = "scalar-command"
	EnvironmentStatic    UnsupportedLayoutReason = "environment-static"
	MixedStaticSources   UnsupportedLayoutReason = "mixed-static-sources"
	StaticOutsideProject UnsupportedLayoutReason = "static-outside-project"
)
