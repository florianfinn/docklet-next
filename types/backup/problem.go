// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original volume backup lifecycle contract.

package backup

import (
	"reflect"
	"strings"
)

// Problem carries stable codes and safe diagnostics; raw engine details stay server-side.
type Problem struct {
	Status     int      `json:"status"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	Code       string   `json:"code" enum:"volume.backup.busy,volume.backup.interrupted,volume.backup.restart-failed,volume.backup.retention-failed,volume.backup.policy-single,volume.backup.policy-invalid,volume.backup.repository-unavailable,volume.backup.engine-unavailable,volume.backup.engine-timeout,volume.backup.engine-output-invalid,volume.backup.repository-check-failed,volume.backup.upload-too-large,volume.backup.upload-invalid,volume.backup.download-too-large,volume.backup.listing-too-large,volume.backup.snapshot-not-found,volume.backup.safety-pinned,volume.backup.writer-unconfirmed,volume.restore.self-mounted,volume.restore.safety-failed,volume.restore.failed-partial,volume.restore.interrupted,volume.restore.interrupted-before-write,volume.restore.restart-failed,backup.target.path-missing,backup.target.not-mounted,backup.target.permission-denied,backup.target.mount-failed,backup.target.unreachable,backup.target.unknown,backup.target.wrong-host,backup.target.retired,backup.target.in-use,backup.target.hosts-unknown,backup.target.duplicate,backup.target.invalid,backup.target.name-exists,backup.target.stale-set,backup.target.key-mismatch,backup.target.sync-failed,backup.target.conflict,backup.target.immutable"`
	ActivityID string   `json:"activityId,omitempty"`
	Reason     string   `json:"reason,omitempty" enum:"format,path,duplicate,entry-type,link,too-many-entries,expanded-too-large,insufficient-space"`
	Volumes    []string `json:"volumes,omitempty"`
	Hosts      []string `json:"hosts,omitempty"`
	Cause      error    `json:"-"`
}

func (e *Problem) Error() string  { return e.Detail }
func (e *Problem) Unwrap() error  { return e.Cause }
func (e *Problem) GetStatus() int { return e.Status }
func (e *Problem) ContentType(contentType string) string {
	switch contentType {
	case "application/json":
		return "application/problem+json"
	case "application/cbor":
		return "application/problem+cbor"
	}
	return contentType
}

// NewProblem deliberately derives public text from a stable code, never a cause.
func NewProblem(status int, code string, cause error) *Problem {
	return &Problem{Status: status, Title: "Volume backup operation refused", Detail: code, Code: code, Cause: cause}
}

const MaxVolumeUploadBytes int64 = 4 << 30
const MaxEdgeDownloadBytes int64 = 512 << 20
const MaxVolumeResponseBytes = 8 << 20
const MaxBrowseEntries = 5000

const MaxExpandedUploadBytes int64 = 16 << 30

// IsProblemCode uses the wire enum as the only diagnostic allowlist.
func IsProblemCode(code string) bool {
	field, _ := reflect.TypeFor[Problem]().FieldByName("Code")
	for _, allowed := range strings.Split(field.Tag.Get("enum"), ",") {
		if code == allowed {
			return true
		}
	}
	return false
}
