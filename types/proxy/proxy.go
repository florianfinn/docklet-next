// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

package proxy

import "time"

type SourceKind string

const (
	ProjectSource     SourceKind = "project"
	NamedVolume       SourceKind = "volume"
	BindSource        SourceKind = "bind"
	SingleFileSource  SourceKind = "single-file"
	UnknownSourceKind SourceKind = "unknown"
)

type Authority string

const (
	FileAuthority      Authority = "file"
	CaddyfileAuthority Authority = "caddyfile"
	AdminAuthority     Authority = "admin-api"
)

type MountMode string

const (
	MountReadWrite MountMode = "rw"
	MountReadOnly  MountMode = "ro"
)

type RouteDirectoryHint struct {
	Value string `json:"value"`
	Fixed bool   `json:"fixed"`
}
type SourceEligibility struct {
	Selectable     bool                `json:"selectable"`
	Reason         Reason              `json:"reason,omitempty"`
	RouteDirectory *RouteDirectoryHint `json:"routeDirectory,omitempty"`
	RootFile       string              `json:"rootFile,omitempty"`
}

// MountMode is display metadata; Eligibility alone controls source selection.
type Source struct {
	MountMode   MountMode          `json:"mountMode,omitempty" enum:"rw,ro"`
	ProjectPath string             `json:"projectPath,omitempty"`
	Eligibility *SourceEligibility `json:"eligibility,omitempty"`
	Kind        SourceKind         `json:"kind" enum:"project,volume,bind,single-file,unknown"`
	Canonical   string             `json:"canonical"`
	MountTarget string             `json:"mountTarget"`
	ReadOnly    bool               `json:"readOnly"`
	Reason      Reason             `json:"reason,omitempty"`
}
type Identity struct {
	ContainerID   string `json:"containerId"`
	ContainerName string `json:"containerName"`
	StartedAt     string `json:"startedAt"`
}
type Eligibility struct {
	Writable bool   `json:"writable"`
	TextOnly bool   `json:"textOnly"`
	Reason   Reason `json:"reason,omitempty"`
	Path     string `json:"path,omitempty"`
	Line     int    `json:"line,omitempty"`
	Detail   string `json:"detail,omitempty"`
}
type Instance struct {
	ID             string      `json:"id"`
	EnvironmentID  string      `json:"environmentId"`
	Name           string      `json:"name"`
	Kind           Kind        `json:"kind" enum:"traefik,caddy"`
	Identity       Identity    `json:"identity"`
	Source         Source      `json:"source"`
	Authority      Authority   `json:"authority" enum:"file,caddyfile,admin-api"`
	RootFile       string      `json:"rootFile,omitempty"`
	RouteDirectory string      `json:"routeDirectory"`
	Generation     string      `json:"generation"`
	Registration   string      `json:"registration"`
	Eligibility    Eligibility `json:"eligibility"`
}
type Settings struct {
	Name           string `json:"name"`
	RootFile       string `json:"rootFile,omitempty"`
	RouteDirectory string `json:"routeDirectory"`
}
type Discovery struct {
	Identity   Identity `json:"identity"`
	Kind       Kind     `json:"kind" enum:"traefik,caddy"`
	Image      string   `json:"image"`
	Version    string   `json:"version,omitempty"`
	Sources    []Source `json:"sources"`
	InstanceID string   `json:"instanceId,omitempty"`
}
type EntryPoint struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	TLS     bool   `json:"tls"`
}
type Inventory struct {
	ResolverSummaries []ResolverSummary `json:"resolverSummaries,omitempty"`
	Generation        string            `json:"generation"`
	Identity          Identity          `json:"identity"`
	Eligibility       Eligibility       `json:"eligibility"`
	EntryPoints       []EntryPoint      `json:"entryPoints"`
	Resolvers         []string          `json:"resolvers"`
	Providers         []string          `json:"providers"`
	Plugins           []string          `json:"plugins"`
	FileProvider      string            `json:"fileProvider,omitempty"`
	Watch             bool              `json:"watch"`
	ThrottleMillis    int64             `json:"throttleMillis"`
	RootFile          string            `json:"rootFile,omitempty"`
	Imports           []string          `json:"imports"`
	AdminAddress      string            `json:"adminAddress,omitempty"`
	AdminOff          bool              `json:"adminOff"`
	Resume            bool              `json:"resume"`
	PersistConfig     bool              `json:"persistConfig"`
	RunningDiverged   bool              `json:"runningDiverged"`
	ImportLine        string            `json:"importLine,omitempty"`
	ImportPresent     bool              `json:"importPresent"`
}
type File struct {
	Path             string    `json:"path"`
	Hash             string    `json:"hash"`
	Content          string    `json:"content"`
	Mode             uint32    `json:"mode"`
	ModifiedAt       time.Time `json:"modifiedAt"`
	Owned            bool      `json:"owned"`
	LastVerifiedHash string    `json:"lastVerifiedHash,omitempty"`
}

// OpaqueFile cannot carry payload, hash or definition names.
type OpaqueFile struct {
	Path string `json:"path"`
}
type Reference struct {
	Name string `json:"name"`
	Kind string `json:"kind" enum:"local,plugin,internal,external,opaque,unresolved"`
	File string `json:"file,omitempty"`
}
type Route struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	File          string      `json:"file"`
	Class         string      `json:"class" enum:"editable,text-only,infrastructure,label-read-only"`
	Eligibility   Eligibility `json:"eligibility"`
	Rule          string      `json:"rule"`
	Priority      int         `json:"priority"`
	Service       string      `json:"service"`
	ServiceFile   string      `json:"serviceFile,omitempty"`
	Upstreams     []string    `json:"upstreams"`
	EntryPoints   []string    `json:"entryPoints"`
	References    []Reference `json:"references"`
	Dependants    []string    `json:"dependants"`
	ProjectID     string      `json:"projectId,omitempty"`
	ContainerName string      `json:"containerName,omitempty"`
	// TLS and Resolver summarise a Traefik router's tls section; Caddy leaves them empty.
	TLS      bool   `json:"tls,omitempty"`
	Resolver string `json:"resolver,omitempty"`
}
type ReadResult struct {
	Generation  string       `json:"generation"`
	Inventory   Inventory    `json:"inventory"`
	Files       []File       `json:"files"`
	OpaqueFiles []OpaqueFile `json:"opaqueFiles"`
	Routes      []Route      `json:"routes"`
	Eligibility Eligibility  `json:"eligibility"`
	// Middlewares lists definitions readable in the graph for selection (Traefik).
	Middlewares []Reference `json:"middlewares,omitempty"`
}
type FileState struct {
	Present bool   `json:"present"`
	Hash    string `json:"hash"`
}
type Change struct {
	Kind        string   `json:"kind" enum:"create-route,edit-route,delete-route,edit-service,insert-import,restore-owned"`
	RouteID     string   `json:"routeId,omitempty"`
	Name        string   `json:"name,omitempty"`
	Service     string   `json:"service,omitempty"`
	Rule        string   `json:"rule,omitempty"`
	Priority    *int     `json:"priority,omitempty"`
	Upstreams   []string `json:"upstreams,omitempty"`
	EntryPoints []string `json:"entryPoints,omitempty"`
	Middlewares []string `json:"middlewares,omitempty"`
	Resolver    string   `json:"resolver,omitempty"`
	Advanced    string   `json:"advanced,omitempty"`
}
type Acknowledgements struct {
	UnresolvedReferences []string `json:"unresolvedReferences"`
	ReplaceRunningConfig bool     `json:"replaceRunningConfig"`
	Restart              bool     `json:"restart"`
}
type ApplyRequest struct {
	Kind             OperationKind     `json:"kind,omitempty" enum:"route"`
	ID               string            `json:"id" minLength:"1" maxLength:"128"`
	Instance         Instance          `json:"instance"`
	Generation       string            `json:"generation"`
	Registration     string            `json:"registration"`
	Target           string            `json:"target"`
	Expected         FileState         `json:"expected"`
	GraphHashes      map[string]string `json:"graphHashes"`
	Change           Change            `json:"change"`
	Acknowledgements Acknowledgements  `json:"acknowledgements"`
}

// AdoptionReceipt is the payload of an adoption record: the container the instance followed.
type AdoptionReceipt struct {
	PreviousContainerID string `json:"previousContainerId" minLength:"1" maxLength:"128"`
	ContainerID         string `json:"containerId" minLength:"1" maxLength:"128"`
	StartedAt           string `json:"startedAt" maxLength:"64"`
}

// AdoptionOutcome answers an adoption request: the registration is current, followed its recreated container, or stays
// refused with a reason.
type AdoptionOutcome string

const (
	AdoptionCurrent AdoptionOutcome = "current"
	AdoptionAdopted AdoptionOutcome = "adopted"
	AdoptionRefused AdoptionOutcome = "refused"
)

// AdoptionReason names why a recreated container was not adopted.
type AdoptionReason string

const (
	AdoptionContainerMissing AdoptionReason = "container-missing"
	AdoptionNameChanged      AdoptionReason = "name-changed"
	AdoptionSourceChanged    AdoptionReason = "source-changed"
	AdoptionModeChanged      AdoptionReason = "mode-changed"
	AdoptionKindChanged      AdoptionReason = "kind-changed"
	AdoptionOperationOpen    AdoptionReason = "operation-open"
	AdoptionAmbiguous        AdoptionReason = "ambiguous"
	AdoptionHeldByInstance   AdoptionReason = "held-by-instance"
	AdoptionModuleOff        AdoptionReason = "module-off"
)

// Adoption is the result of POST …/instances/{instanceId}/adopt.
type Adoption struct {
	Outcome      AdoptionOutcome `json:"outcome" enum:"current,adopted,refused"`
	Reason       AdoptionReason  `json:"reason,omitzero" enum:"container-missing,name-changed,source-changed,mode-changed,kind-changed,operation-open,ambiguous,held-by-instance,module-off"`
	Instance     Instance        `json:"instance"`
	SupersededBy string          `json:"supersededBy,omitzero" maxLength:"128"`
	OperationID  string          `json:"operationId,omitzero" maxLength:"128"`
}

type Operation struct {
	Kind         OperationKind           `json:"kind,omitempty"`
	Adoption     *AdoptionReceipt        `json:"adoption,omitempty"`
	Resolver     *ResolverReceipt        `json:"resolver,omitempty"`
	Certificate  *CertificateObservation `json:"certificate,omitempty"`
	Probe        *ProbeEvidence          `json:"probe,omitempty"`
	ID           string                  `json:"id"`
	InstanceID   string                  `json:"instanceId"`
	Generation   string                  `json:"generation"`
	Registration string                  `json:"registration"`
	Digest       string                  `json:"digest"`
	State        State                   `json:"state" enum:"pending,verified,applied-unverified,rejected,restored,recovery-required,released,not-applied,applied,rolled-back,refused"`
	Phase        Phase                   `json:"phase" enum:"admitted,staging,publishing,reloading,restarting,observing,marking,restoring,backup,writing,recreating,applied,rolled-back,refused,recovery-required"`
	Reason       Reason                  `json:"reason,omitempty"`
	Evidence     EvidenceKind            `json:"evidence,omitempty" enum:"routing,configuration,load"`
	Target       string                  `json:"target"`
	Old          FileState               `json:"old"`
	New          FileState               `json:"new"`
	Current      *FileState              `json:"current,omitempty"`
	CreatedAt    time.Time               `json:"createdAt"`
	FinishedAt   *time.Time              `json:"finishedAt,omitempty"`
	ReleasedBy   string                  `json:"releasedBy,omitempty"`
	// Diagnostic names where an engine refused or failed (validator message, file and line).
	Diagnostic *Diagnostic `json:"diagnostic,omitempty"`
}

// Diagnostic is bounded engine text; it never contains opaque file content.
type Diagnostic struct {
	Path   string `json:"path,omitempty" maxLength:"1024"`
	Line   int    `json:"line,omitempty" minimum:"0"`
	Detail string `json:"detail,omitempty" maxLength:"4096"`
}
type Query struct {
	Kind         OperationKind `json:"kind,omitempty"`
	ID           string        `json:"id"`
	Instance     Instance      `json:"instance"`
	Generation   string        `json:"generation"`
	Registration string        `json:"registration"`
}
type FenceResult struct {
	Generation string     `json:"generation"`
	Fenced     bool       `json:"fenced"`
	Operation  *Operation `json:"operation,omitempty"`
}
type ReleaseRequest struct {
	Query
	Confirmed FileState `json:"confirmed"`
	Actor     string    `json:"actor"`
}

// OperationRequest is the web request. The hub supplies the ID, registered
// instance and executor binding; clients never manufacture those fields.
type OperationRequest struct {
	Target           string            `json:"target"`
	Expected         FileState         `json:"expected"`
	GraphHashes      map[string]string `json:"graphHashes"`
	Change           Change            `json:"change"`
	Acknowledgements Acknowledgements  `json:"acknowledgements"`
}
type CreateInstanceRequest struct {
	EnvironmentID  string `json:"environmentId"`
	ContainerID    string `json:"containerId"`
	Kind           Kind   `json:"kind" enum:"traefik,caddy"`
	MountTarget    string `json:"mountTarget"`
	Name           string `json:"name" maxLength:"128"`
	RootFile       string `json:"rootFile,omitempty"`
	RouteDirectory string `json:"routeDirectory"`
}
