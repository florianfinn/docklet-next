// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125. M24 recovery evidence, validation and reason contracts.

package activity

import (
	"bytes"
	"encoding/hex"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"io"
	"math"
	"net/url"
	"reflect"
	"strings"
	"unicode/utf8"
)

const RuntimeMaxBodyBytes = 1 << 20
const RuntimeMaxOutputBytes = 256 << 10

var ErrRuntimeWire = errors.New("invalid runtime response")

// Only an authenticated Execute busy/cancelling response is no-worker authority.
// Generic conflicts, storage errors and absent Cancel never carry this body.
type RuntimeExecutionRefusal struct {
	Code Code `json:"code" enum:"container.recreate-recovered,container.recreate-ambiguous,container.origin-locked,runtime.idempotency-mismatch,run.request-mismatch,runtime.outcome-unknown,runtime.interrupted,runtime.cancelled,runtime.failed,runtime.wait-timeout,runtime.waiter-disconnected,runtime.parent-unresolved,runtime.target-removed,runtime.target-changed,runtime.target-unavailable,run.started,run.joined,run.fresh,run.busy,run.cancelling,activity.queued,activity.running,activity.success,activity.failed,activity.cancelled,activity.message,deploy.pull,job.queued,job.waiting,job.running,job.retrying,job.succeeded,job.partial,job.skipped,job.failed,job.needs_attention,job.canceled,container.die,container.oom,container.rename,container.unhealthy,image.untag,image.import,image.prune,volume.prune,network.prune,container.start,container.stop,container.restart,container.delete,container.create,container.scan,container.update,container.deploy,container.kill,container.pause,container.unpause,container.error,image.pull,image.load,image.tag,image.commit,image.delete,image.scan,image.error,image.vulnerability_scan,project.deploy,project.delete,project.start,project.stop,project.create,project.update,project.error,volume.create,volume.rename,volume.delete,volume.error,volume.file.create,volume.file.delete,volume.file.upload,volume.file.update,volume.workspace.update,volume.backup.create,volume.backup.delete,volume.backup.restore,volume.backup.restore_files,volume.backup.download,volume.backup.target.key_viewed,network.create,network.delete,network.connect,network.disconnect,network.error,system.prune,user.login,user.logout,system.auto_update,system.upgrade,environment.create,environment.connect,environment.disconnect,environment.update,environment.delete,environment.api_key.regenerated,environment.mtls.ca_generated,environment.mtls.cert_issued,environment.mtls.enroll,environment.mtls.download,notification.send,autoheal.restarted,autoheal.restart-failed,autoheal.limit-reached,autoheal.interrupted,notification.delivery-failed,load.sustained,container.condition.down,container.condition.unhealthy,container.condition.restart-loop,container.condition.running-again,update.self-failed,update.outcome-unknown"`
}

func (r RuntimeExecutionRefusal) Safe() bool {
	return r.Code == CodeRunBusy || r.Code == CodeRunCancelling
}

type RuntimeCancellation struct {
	Identity    RuntimeRunIdentity `json:"identity"`
	Attribution RuntimeAttribution `json:"attribution"`
}

// RuntimeIdentityQuery is the sole GET identity encoding. RunID is in the path;
// the four required query values are exact, unique and contain no credentials.
func RuntimeIdentityQuery(id RuntimeRunIdentity) string {
	return url.Values{"managerId": {id.ManagerID}, "registrationId": {id.RegistrationID}, "executorGeneration": {id.ExecutorGeneration}, "requestDigest": {id.RequestDigest}}.Encode()
}
func ParseRuntimeIdentityQuery(runID, raw string) (RuntimeRunIdentity, error) {
	if len(raw) > 2048 {
		return RuntimeRunIdentity{}, ErrRuntimeWire
	}
	v, err := url.ParseQuery(raw)
	if err != nil || len(v) != 4 {
		return RuntimeRunIdentity{}, ErrRuntimeWire
	}
	for _, key := range []string{"managerId", "registrationId", "executorGeneration", "requestDigest"} {
		if len(v[key]) != 1 || v.Get(key) == "" {
			return RuntimeRunIdentity{}, ErrRuntimeWire
		}
	}
	id := RuntimeRunIdentity{RuntimeExecutorIdentity: RuntimeExecutorIdentity{ManagerID: v.Get("managerId"), RegistrationID: v.Get("registrationId"), ExecutorGeneration: v.Get("executorGeneration")}, RunID: runID, RequestDigest: v.Get("requestDigest")}
	if !RuntimeIdentityValid(id) {
		return RuntimeRunIdentity{}, ErrRuntimeWire
	}
	return id, nil
}
func runtimeUUIDWire(s string) bool {
	if len(s) != 36 || s != strings.ToLower(s) {
		return false
	}
	for _, n := range []int{8, 13, 18, 23} {
		if s[n] != '-' {
			return false
		}
	}
	b, err := hex.DecodeString(strings.ReplaceAll(s, "-", ""))
	return err == nil && len(b) == 16
}
func RuntimeIdentityValid(id RuntimeRunIdentity) bool {
	b, err := hex.DecodeString(id.RequestDigest)
	return runtimeUUIDWire(id.RunID) && runtimeUUIDWire(id.ExecutorGeneration) && err == nil && len(b) == 32 && strings.ToLower(id.RequestDigest) == id.RequestDigest && len(id.ManagerID) > 0 && len(id.ManagerID) <= 256 && len(id.RegistrationID) > 0 && len(id.RegistrationID) <= 256
}

// The v2 decoder rejects duplicate members, unknown members, malformed UTF-8 and
// trailing values. Explicit null cannot silently become a scalar zero value.
func DecodeRuntimeJSON(body []byte, value any) error {
	if len(body) == 0 || len(body) > RuntimeMaxBodyBytes {
		return ErrRuntimeWire
	}
	decoder := jsontext.NewDecoder(bytes.NewReader(body))
	for {
		_, err := decoder.ReadToken()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || decoder.StackDepth() > 32 {
			return ErrRuntimeWire
		}
	}
	if err := json.Unmarshal(body, value, json.RejectUnknownMembers(true)); err != nil {
		return ErrRuntimeWire
	}
	if err := runtimeNullShape(body, reflect.TypeOf(value).Elem()); err != nil {
		return err
	}
	return nil
}
func runtimeNullShape(body []byte, typ reflect.Type) error {
	if typ.Kind() == reflect.Pointer {
		if string(body) == "null" {
			return nil
		}
		return runtimeNullShape(body, typ.Elem())
	}
	if string(body) == "null" {
		if typ.Kind() == reflect.Slice || typ.Kind() == reflect.Map || typ.Kind() == reflect.Interface {
			return nil
		}
		return ErrRuntimeWire
	}
	if typ.Kind() != reflect.Struct {
		return nil
	}
	// Time and custom scalar encodings are validated by their own unmarshaler.
	if typ.PkgPath() == "time" {
		return nil
	}
	var fields map[string]jsontext.Value
	if err := json.Unmarshal(body, &fields); err != nil || fields == nil {
		return ErrRuntimeWire
	}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		tag := f.Tag.Get("json")
		name, _, _ := strings.Cut(tag, ",")
		if name == "-" {
			continue
		}
		if f.Anonymous && name == "" {
			if err := runtimeNullShape(body, f.Type); err != nil {
				return err
			}
			continue
		}
		if name == "" {
			name = f.Name
		}
		raw, ok := fields[name]
		if !ok {
			if !strings.Contains(tag, "omitempty") && !strings.Contains(tag, "omitzero") {
				return ErrRuntimeWire
			}
			continue
		}
		if err := runtimeNullShape(raw, f.Type); err != nil {
			return err
		}
	}
	return nil
}
func ValidateRuntimeExecutionDetail(d *RuntimeExecutionDetail, runID string) error {
	if d == nil || d.Activity.ID != runID || d.Activity.EnvironmentID != "0" || d.RetainedStartSeq == 0 || d.RetainedStartSeq > math.MaxInt64 || len(d.Messages) > 100 || len(d.StepTrace) > 256 || d.Output == nil || d.Messages == nil || d.StepTrace == nil {
		return ErrRuntimeWire
	}
	if d.Activity.OutcomeState != RuntimeOutcomeKnown {
		return ErrRuntimeWire
	}
	switch d.Activity.Status {
	case StatusQueued, StatusRunning, StatusSuccess, StatusFailed, StatusCancelled:
	default:
		return ErrRuntimeWire
	}
	if d.Activity.Progress != nil && (*d.Activity.Progress < 0 || *d.Activity.Progress > 100) {
		return ErrRuntimeWire
	}
	for _, step := range d.StepTrace {
		if len(step.Name) > 4096 || step.StartedAt.IsZero() {
			return ErrRuntimeWire
		}
		switch step.Status {
		case StatusQueued, StatusRunning, StatusSuccess, StatusFailed, StatusCancelled:
		default:
			return ErrRuntimeWire
		}
	}
	if d.Activity.StartedAt.IsZero() || d.Activity.CreatedAt.IsZero() {
		return ErrRuntimeWire
	}
	for _, m := range d.Messages {
		switch m.Level {
		case MessageLevelInfo, MessageLevelWarning, MessageLevelError, MessageLevelSuccess:
		default:
			return ErrRuntimeWire
		}
		if m.ActivityID != runID || m.ID == "" || len(m.Message) > 65536 || !utf8.ValidString(m.Message) {
			return ErrRuntimeWire
		}
	}
	if d.RetainedStartSeq > 1 && !d.OutputTruncated {
		return ErrRuntimeWire
	}
	var previous uint64
	for i, line := range d.Output {
		if line.OutputSeq == 0 || line.OutputSeq >= math.MaxInt64 || (i == 0 && line.OutputSeq != d.RetainedStartSeq) || (i > 0 && line.OutputSeq <= previous) || !utf8.ValidString(line.Text) || line.Timestamp.IsZero() {
			return ErrRuntimeWire
		}
		if i > 0 && line.OutputSeq > previous+1 && !d.OutputTruncated {
			return ErrRuntimeWire
		}
		previous = line.OutputSeq
	}
	output, err := json.Marshal(d.Output)
	if err != nil || len(output) > RuntimeMaxOutputBytes {
		return ErrRuntimeWire
	}
	body, err := json.Marshal(d)
	if err != nil || len(body) > RuntimeMaxBodyBytes-1024 {
		return ErrRuntimeWire
	}
	return nil
}
func ValidateRuntimeOutcome(o RuntimeExecutionOutcome, identity RuntimeRunIdentity) error {
	if o.Identity != identity || !RuntimeIdentityValid(identity) {
		return ErrRuntimeWire
	}
	if o.State == RuntimeAnswerNotReceived {
		if o.Detail != nil {
			return ErrRuntimeWire
		}
		return nil
	}
	if o.State != RuntimeAnswerActive && o.State != RuntimeAnswerTerminal {
		return ErrRuntimeWire
	}
	if err := ValidateRuntimeExecutionDetail(o.Detail, identity.RunID); err != nil {
		return err
	}
	if o.Detail.Activity.Result != nil {
		result := o.Detail.Activity.Result
		allowed := o.Detail.Activity.Status == StatusSuccess
		if o.Detail.Activity.Status == StatusFailed && o.Detail.Activity.Type == TypeContainerRedeploy && o.Detail.Activity.Operation == RuntimeUpdateContainer {
			item := o.Detail.Activity
			allowed = (item.MessageCode == string(CodeRuntimeInterrupted) || (item.MessageCode == string(CodeRuntimeFailed) && item.ImageLifecycle != nil && item.ImageLifecycle.Code == "update.target-changed")) && (item.ImageLifecycle == nil || !item.ImageLifecycle.Verified)
		}
		if !allowed || o.Detail.Activity.ResourceID == nil || result.OldID != *o.Detail.Activity.ResourceID || len(result.NewID) != 64 || result.NewID == result.OldID || result.OldName == "" || len(result.OldName) > 256 || result.NewName == "" || len(result.NewName) > 256 {
			return ErrRuntimeWire
		}
	}
	if o.Detail.Activity.Status == StatusSuccess && (o.Detail.Activity.Type == TypeContainerEdit || o.Detail.Activity.Type == TypeContainerRedeploy) && o.Detail.Activity.Result == nil {
		return ErrRuntimeWire
	}
	terminal := o.Detail.Activity.Status == StatusSuccess || o.Detail.Activity.Status == StatusFailed || o.Detail.Activity.Status == StatusCancelled
	if terminal != (o.State == RuntimeAnswerTerminal) || (terminal && o.Detail.Activity.EndedAt == nil) {
		return ErrRuntimeWire
	}
	return nil
}
