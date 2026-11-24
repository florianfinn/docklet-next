// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original proxy module API and lifecycle wiring.

package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/types/base"
	moduledto "github.com/florianfinn/docklet-next/types/modules"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"net/url"
)

const hostPrefix = "/api/environments/0/modules/proxy"

type HostTransport struct {
	Hosts *environment.EnvironmentService
	Local *Module
}

func (t *HostTransport) Registration(ctx context.Context, id string) (string, error) {
	if id == "0" {
		return "local", nil
	}
	e, err := t.Hosts.GetEnvironmentByID(ctx, id)
	if err != nil {
		return "", err
	}
	return environmentRegistration(e), nil
}
func environmentRegistration(e *environment.Environment) string {
	b := fmt.Sprintf("%s\x00%s\x00%d\x00%t", e.ID, e.ApiUrl, e.CredentialRevision, e.IsEdge)
	h := sha256.Sum256([]byte(b))
	return hex.EncodeToString(h[:])
}
func (t *HostTransport) call(ctx context.Context, env, method, p string, in, out any) error {
	b, err := marshal(in)
	if err != nil {
		return err
	}
	if in == nil {
		b = nil
	}
	body, status, err := t.Hosts.ProxyRequest(ctx, env, method, hostPrefix+p, b)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		var moduleProblem moduledto.Problem
		if json.Unmarshal(body, &moduleProblem) == nil && moduleProblem.Feature != "" && moduleProblem.Code != "" {
			return &moduleProblem
		}
		var problem dto.Problem
		if json.Unmarshal(body, &problem) == nil && problem.Code != "" {
			return &problem
		}
		return dto.Fail(dto.AgentUnavailable, 502)
	}
	if err = json.Unmarshal(body, out); err != nil {
		return dto.Fail(dto.InvalidInput, 502)
	}
	return nil
}
func (t *HostTransport) Apply(ctx context.Context, q dto.ApplyRequest) (dto.Operation, error) {
	if q.Instance.EnvironmentID == "0" {
		return t.Local.Agent.Apply(ctx, q)
	}
	var r base.ApiResponse[dto.Operation]
	err := t.call(ctx, q.Instance.EnvironmentID, "POST", "/apply", q, &r)
	return r.Data, err
}
func (t *HostTransport) Query(ctx context.Context, q dto.Query) (dto.Operation, error) {
	if q.Instance.EnvironmentID == "0" {
		return t.Local.Agent.Query(ctx, q)
	}
	var r base.ApiResponse[dto.Operation]
	p := "/operations/" + url.PathEscape(q.ID) + "?generation=" + url.QueryEscape(q.Generation) + "&registration=" + url.QueryEscape(q.Registration) + "&instanceId=" + url.QueryEscape(q.Instance.ID)
	err := t.call(ctx, q.Instance.EnvironmentID, "GET", p, nil, &r)
	return r.Data, err
}
func (t *HostTransport) Fence(ctx context.Context, q dto.Query) (dto.FenceResult, error) {
	if q.Instance.EnvironmentID == "0" {
		return t.Local.Agent.Fence(ctx, q)
	}
	var r base.ApiResponse[dto.FenceResult]
	err := t.call(ctx, q.Instance.EnvironmentID, "POST", "/fence", q, &r)
	return r.Data, err
}
func (t *HostTransport) Release(ctx context.Context, q dto.ReleaseRequest) (dto.Operation, error) {
	if q.Instance.EnvironmentID == "0" {
		return t.Local.Agent.Release(ctx, q)
	}
	var r base.ApiResponse[dto.Operation]
	err := t.call(ctx, q.Instance.EnvironmentID, "POST", "/release", q, &r)
	return r.Data, err
}
func (t *HostTransport) Discover(ctx context.Context, env string) ([]dto.Discovery, error) {
	if env == "0" {
		return t.Local.DiscoverHost(ctx)
	}
	var r base.ApiResponse[[]dto.Discovery]
	err := t.call(ctx, env, "GET", "/discover", nil, &r)
	return r.Data, err
}
func (t *HostTransport) Inventory(ctx context.Context, i dto.Instance) (dto.Inventory, error) {
	if i.EnvironmentID == "0" {
		return t.Local.Inventory(ctx, i)
	}
	var r base.ApiResponse[dto.Inventory]
	err := t.call(ctx, i.EnvironmentID, "POST", "/inventory", i, &r)
	return r.Data, err
}

func (t *HostTransport) CreateResolver(ctx context.Context, q dto.ResolverApplyRequest) (dto.Operation, error) {
	if q.Instance.EnvironmentID == "0" {
		return t.Local.Agent.CreateResolver(ctx, q)
	}
	var r base.ApiResponse[dto.Operation]
	err := t.call(ctx, q.Instance.EnvironmentID, "POST", "/resolvers", q, &r)
	// Network failures cannot carry submitted values to callers or loggers.
	if err != nil {
		if p, ok := errors.AsType[*moduledto.Problem](err); ok {
			return r.Data, &moduledto.Problem{Status: p.Status, Code: p.Code, Feature: p.Feature}
		}
		if p, ok := errors.AsType[*dto.Problem](err); ok {
			return r.Data, dto.Fail(p.Code, p.Status)
		}
		return r.Data, dto.Fail(dto.AgentUnavailable, 503)
	}
	return r.Data, nil
}

func (t *HostTransport) CheckCertificate(ctx context.Context, q dto.HostCertificateCheckRequest) (dto.CertificateObservation, error) {
	if q.Instance.EnvironmentID == "0" {
		return t.Local.Agent.CheckCertificate(ctx, q)
	}
	var r base.ApiResponse[dto.CertificateObservation]
	err := t.call(ctx, q.Instance.EnvironmentID, "POST", "/certificate-check", q, &r)
	return r.Data, err
}
