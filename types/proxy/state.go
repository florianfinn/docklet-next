// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

// Package proxy defines the shared proxy module wire contract.
package proxy

import "fmt"

type Kind string

const (
	Traefik Kind = "traefik"
	Caddy   Kind = "caddy"
)

type State string

const (
	Pending           State = "pending"
	Applied           State = "applied"
	RolledBack        State = "rolled-back"
	Refused           State = "refused"
	Verified          State = "verified"
	AppliedUnverified State = "applied-unverified"
	Rejected          State = "rejected"
	Restored          State = "restored"
	RecoveryRequired  State = "recovery-required"
	Released          State = "released"
	NotApplied        State = "not-applied"
)

func (s State) Final() bool {
	switch s {
	case Verified, AppliedUnverified, Rejected, Restored, Released, NotApplied, Applied, RolledBack, Refused:
		return true
	}
	return false
}

// Valid retains the v19 route state set.
func (s State) Valid() bool {
	switch s {
	case Pending, Verified, AppliedUnverified, Rejected, Restored, RecoveryRequired, Released, NotApplied:
		return true
	}
	return false
}
func (s State) ValidFor(kind OperationKind) bool {
	switch kind.Effective() {
	case RouteOperation:
		return s.Valid()
	case ResolverOperation:
		switch s {
		case Pending, Applied, RolledBack, Refused, RecoveryRequired, Released, NotApplied:
			return true
		}
	case AdoptionOperation:
		// An adoption is recorded finished in the transaction that adopts (#592).
		return s == Applied
	}
	return false
}

type Phase string

const (
	Admitted   Phase = "admitted"
	Staging    Phase = "staging"
	Publishing Phase = "publishing"
	Reloading  Phase = "reloading"
	Restarting Phase = "restarting"
	Observing  Phase = "observing"
	Marking    Phase = "marking"
	Restoring  Phase = "restoring"
)

func (p Phase) Valid() bool {
	switch p {
	case Admitted, Staging, Publishing, Reloading, Restarting, Observing, Marking, Restoring:
		return true
	}
	return false
}

type EvidenceKind string

const (
	Routing       EvidenceKind = "routing"
	Configuration EvidenceKind = "configuration"
	Load          EvidenceKind = "load"
)

type Reason string

const (
	SourceIneligible           Reason = "source-ineligible"
	RouteDirectoryMismatch     Reason = "route-directory-mismatch"
	ProviderDirectoryElsewhere Reason = "provider-directory-elsewhere"
	CaddyConfigImplicit        Reason = "caddy-config-implicit"
	ReadOnly                   Reason = "read-only"
	RouteChanged               Reason = "route-changed"
	CertificateUnavailable     Reason = "certificate-unavailable"
	ProbeImageUnavailable      Reason = "probe-image-unavailable"
	ProbeClientMissing         Reason = "probe-client-missing"
	ProbeClientUnsupported     Reason = "probe-client-unsupported"
	ThirdVersion               Reason = "third-version"
	BackupInvalid              Reason = "backup-invalid"
	UnsupportedLayout          Reason = "unsupported-layout"
	EnvConflict                Reason = "env-conflict"
	EnvKeyExists               Reason = "env-key-exists"
	EnvDuplicate               Reason = "env-duplicate"
	StorageNotWritable         Reason = "storage-not-writable"
	UnknownOperationKind       Reason = "unknown-operation-kind"
	ComposeLoadFailed          Reason = "compose-load-failed"
	ComposeUpFailed            Reason = "compose-up-failed"
	StaticParseFailed          Reason = "static-parse-failed"
	EffectUnsettled            Reason = "effect-unsettled"
	TargetUnclear              Reason = "target-unclear"
	IdentityUnclear            Reason = "identity-unclear"
	Interrupted                Reason = "interrupted"
	NotImplemented             Reason = "not-implemented"
	NoObservableDifference     Reason = "no-observable-difference"
	NotObservable              Reason = "not-observable"
	ContainerChanged           Reason = "container-changed"
	LogsUnavailable            Reason = "logs-unavailable"
	GenerationMismatch         Reason = "generation-mismatch"
	OperationConflict          Reason = "operation-conflict"
	ModuleBusy                 Reason = "module-busy"
	StaleHash                  Reason = "stale-hash"
	SourceReadOnly             Reason = "source-read-only"
	UnknownOperation           Reason = "unknown-operation"
	Fenced                     Reason = "operation-fenced"
	RegistrationChanged        Reason = "registration-changed"
	JournalLost                Reason = "journal-lost"
	InvalidInput               Reason = "invalid-input"
	InputLimit                 Reason = "input-limit"
	ExecFailed                 Reason = "exec-failed"
	ExecRunning                Reason = "exec-running"
	ProxyRejected              Reason = "proxy-rejected"
	RestoreFailed              Reason = "restore-failed"
	InvalidGraph               Reason = "invalid-graph"
	IncompleteInventory        Reason = "incomplete-inventory"
	OpaqueInput                Reason = "opaque-input"
	SingleFileMount            Reason = "single-file-mount"
	RefusedBind                Reason = "refused-bind"
	ReadOnlyMount              Reason = "read-only-mount"
	UnknownSource              Reason = "unknown-source"
	UnsupportedMetadata        Reason = "unsupported-metadata"
	WatchOff                   Reason = "watch-off"
	UnsupportedFormat          Reason = "unsupported-format"
	TemplateInput              Reason = "template-input"
	RuleSyntaxV2               Reason = "rule-syntax-v2"
	SharedAcrossFiles          Reason = "shared-across-files"
	PersistenceOff             Reason = "persistence-off"
	RelativeImport             Reason = "relative-import"
	ImportMissing              Reason = "import-missing"
	AcknowledgementRequired    Reason = "acknowledgement-required"
	ProbeUnavailable           Reason = "probe-unavailable"
	ProbeOptionUnavailable     Reason = "probe-option-unavailable"
	AdminUnavailable           Reason = "admin-unavailable"
	AgentUnavailable           Reason = "agent-unavailable"
	EngineUnavailable          Reason = "engine-unavailable"
	TargetChanged              Reason = "target-changed"
)

// Problem preserves a typed reason across both host transports.
type Problem struct {
	Status      int    `json:"status"`
	Code        Reason `json:"code" enum:"no-observable-difference,not-observable,container-changed,logs-unavailable,generation-mismatch,operation-conflict,module-busy,stale-hash,source-read-only,unknown-operation,operation-fenced,registration-changed,journal-lost,invalid-input,input-limit,exec-failed,exec-running,proxy-rejected,restore-failed,invalid-graph,incomplete-inventory,opaque-input,single-file-mount,refused-bind,read-only-mount,unknown-source,unsupported-metadata,watch-off,unsupported-format,template-input,rule-syntax-v2,shared-across-files,persistence-off,relative-import,import-missing,acknowledgement-required,probe-unavailable,probe-option-unavailable,admin-unavailable,agent-unavailable,engine-unavailable,target-changed,source-ineligible,route-directory-mismatch,provider-directory-elsewhere,caddy-config-implicit,read-only,route-changed,certificate-unavailable,probe-image-unavailable,probe-client-missing,probe-client-unsupported,third-version,backup-invalid,unsupported-layout,env-conflict,env-key-exists,env-duplicate,storage-not-writable,unknown-operation-kind,compose-load-failed,compose-up-failed,static-parse-failed,effect-unsettled,target-unclear,identity-unclear,interrupted,not-implemented"`
	Path        string `json:"path,omitempty"`
	Line        int    `json:"line,omitempty"`
	Detail      string `json:"detail,omitempty"`
	OperationID string `json:"operationId,omitempty"`
	Generation  string `json:"generation,omitempty"`
}

func (p *Problem) Error() string            { return fmt.Sprintf("proxy: %s", p.Code) }
func (p *Problem) GetStatus() int           { return p.Status }
func Fail(code Reason, status int) *Problem { return &Problem{Status: status, Code: code} }

func (p *Problem) ContentType(string) string { return "application/problem+json" }
