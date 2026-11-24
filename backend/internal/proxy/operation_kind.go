// Source: docklet-next/issues/492
// Revision: 52bae4d0610f04e66451664e2c0741699c901873
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 contract foundation; no proxy effects.

package proxy

import (
	"bytes"
	"encoding/json/v2"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

// Discriminate before decoding any payload. An absent kind is the v19 route contract.
func payloadKind(raw []byte) (dto.OperationKind, bool) {
	var envelope struct {
		Kind dto.OperationKind `json:"kind"`
	}
	err := json.Unmarshal(raw, &envelope)
	return envelope.Kind, err == nil && bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{"))
}
func deferredReceipt(o dto.Operation, kind dto.OperationKind) dto.Operation {
	o.Kind = kind.Effective()
	if !o.Phase.Valid() && !dto.ResolverPhase(o.Phase).Valid() {
		o.Phase = dto.Admitted
	}
	// Final release receipts remain readable even when the original payload is unknown.
	if o.State == dto.Released {
		return o
	}
	o.State = dto.RecoveryRequired
	o.Reason = dto.UnknownOperationKind
	o.FinishedAt = nil
	return o
}
func journalReceipt(j Journal) dto.Operation {
	var prior dto.Operation
	_ = json.Unmarshal(j.Result, &prior)
	return dto.Operation{ID: j.ID, InstanceID: prior.InstanceID, Kind: prior.Kind, Adoption: prior.Adoption, Resolver: prior.Resolver, Generation: j.Generation, Registration: j.Registration, Digest: j.Digest, State: j.State, Phase: j.Phase, Target: prior.Target, Old: prior.Old, New: prior.New, Current: prior.Current, ReleasedBy: prior.ReleasedBy, Reason: prior.Reason, CreatedAt: j.CreatedAt, FinishedAt: j.FinishedAt}
}
func recordReceipt(r Record) dto.Operation {
	var prior dto.Operation
	_ = json.Unmarshal(r.Result, &prior)
	return dto.Operation{ID: r.ID, InstanceID: r.InstanceID, Kind: prior.Kind, Adoption: prior.Adoption, Resolver: prior.Resolver, Generation: r.Generation, Registration: r.Registration, Digest: r.Digest, State: r.State, Phase: prior.Phase, Target: prior.Target, Old: prior.Old, New: prior.New, Current: prior.Current, ReleasedBy: prior.ReleasedBy, Reason: prior.Reason, CreatedAt: r.CreatedAt, FinishedAt: r.FinishedAt}
}
func deferredOperation(o dto.Operation) bool {
	return o.Kind.Effective() != dto.RouteOperation && o.Kind != dto.ResolverOperation || o.Reason == dto.UnknownOperationKind
}

// Typed resolver envelopes can be read before the resolver executor is installed.
// Unfinished receipts remain held; only resolver final receipts are settled data.
func decodeResolverPayload(raw, result []byte, id, generation, registration, digest string, state dto.State) (dto.ApplyRequest, dto.Operation, bool) {
	var envelope dto.ResolverEnvelope
	var o dto.Operation
	if json.Unmarshal(raw, &envelope, json.RejectUnknownMembers(true)) != nil || json.Unmarshal(result, &o) != nil || envelope.Intent.Validate() != nil || envelope.Kind != dto.ResolverOperation || envelope.ID != id || envelope.Generation != generation || envelope.Registration != registration || envelope.Instance.ID == "" || o.Kind != dto.ResolverOperation || o.ID != id || o.InstanceID != envelope.Instance.ID || o.Generation != generation || o.Registration != registration || o.Digest != digest || o.State != state || !state.ValidFor(dto.ResolverOperation) || o.Resolver == nil || !o.Resolver.Phase.Valid() {
		return dto.ApplyRequest{}, dto.Operation{}, false
	}
	if o.Resolver.Phase.Final() && dto.State(o.Resolver.Phase) != state {
		return dto.ApplyRequest{}, dto.Operation{}, false
	}
	if o.Resolver.Phase == dto.ResolverRecoveryRequired && state != dto.RecoveryRequired && state != dto.Released {
		return dto.ApplyRequest{}, dto.Operation{}, false
	}
	receiptEnvelope := envelope
	receiptEnvelope.Intent = o.Resolver.Intent
	receiptDigest, err := receiptEnvelope.Digest()
	if err != nil || receiptDigest != digest {
		return dto.ApplyRequest{}, dto.Operation{}, false
	}
	if (state == dto.Applied || state == dto.RolledBack || state == dto.Refused) && dto.State(o.Resolver.Phase) != state {
		return dto.ApplyRequest{}, dto.Operation{}, false
	}
	intentDigest, err := envelope.Digest()
	if err != nil || intentDigest != digest {
		return dto.ApplyRequest{}, dto.Operation{}, false
	}
	q := dto.ApplyRequest{Kind: dto.ResolverOperation, ID: id, Instance: envelope.Instance, Generation: generation, Registration: registration}
	if !dto.ResolverPhase(o.Phase).Valid() || dto.ResolverPhase(o.Phase) != o.Resolver.Phase {
		return dto.ApplyRequest{}, dto.Operation{}, false
	}
	return q, o, true
}

// Adoption is a finished hub receipt, never an executor request or recovery job.
func decodeAdoptionPayload(r Record) (dto.ApplyRequest, dto.Operation, bool) {
	var q dto.ApplyRequest
	var o dto.Operation
	if json.Unmarshal(r.Request, &q) != nil || json.Unmarshal(r.Result, &o) != nil {
		return q, o, false
	}
	a := o.Adoption
	valid := q.Kind == dto.AdoptionOperation && o.Kind == dto.AdoptionOperation && q.ID == r.ID && o.ID == r.ID && q.Instance.ID == r.InstanceID && o.InstanceID == r.InstanceID && q.Generation == r.Generation && o.Generation == r.Generation && q.Registration == r.Registration && o.Registration == r.Registration && r.Digest == "" && o.Digest == "" && r.State == dto.Applied && o.State == dto.Applied && o.Phase == dto.Phase(dto.Applied) && r.FinishedAt != nil && o.FinishedAt != nil && a != nil
	if valid {
		valid = a.PreviousContainerID != "" && a.ContainerID != "" && a.PreviousContainerID != a.ContainerID && a.ContainerID == q.Instance.Identity.ContainerID && a.StartedAt == q.Instance.Identity.StartedAt && o.Target == q.Instance.Identity.ContainerName && o.Target != ""
	}
	return q, o, valid
}
