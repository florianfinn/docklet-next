// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/contract"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
)

var ErrRuntimeTransport = errors.New("runtime outcome unavailable")

// RuntimeRemoteTarget is trusted registration/metadata, never dispatch JSON.
// AgentAccess still validates the current credential/compatibility/session on
// every request. Targets must meet the shared minimum agent contract.
type RuntimeRemoteTarget struct {
	EnvironmentID        string
	Endpoint             string
	Token                string
	Edge                 bool
	AgentContractVersion int
}
type RuntimeRemoteTransport interface {
	Execute(context.Context, activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error)
	Query(context.Context, activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error)
	Cancel(context.Context, activitytypes.RuntimeCancellation) (activitytypes.RuntimeExecutionOutcome, error)
}

type RuntimeExecutionRefused struct{ Code activitytypes.Code }

func (e *RuntimeExecutionRefused) Error() string { return "runtime execution refused" }

type RuntimeSender struct {
	target  RuntimeRemoteTarget
	access  remenv.AgentAccessCheck
	client  *http.Client
	tunnels *edge.TunnelRegistry
}

func NewRuntimeSender(target RuntimeRemoteTarget, access remenv.AgentAccessCheck, client *http.Client, tunnels *edge.TunnelRegistry) (*RuntimeSender, error) {
	if target.EnvironmentID == "" || target.EnvironmentID == "0" || target.Token == "" || access == nil || target.AgentContractVersion < contract.MinimumAgentVersion {
		return nil, remenv.ErrAgentUpdateRequired
	}
	if client == nil {
		client = &http.Client{}
	}
	copyClient := *client
	copyClient.Jar = nil
	copyClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if copyClient.Timeout <= 0 || copyClient.Timeout > 30*time.Minute {
		copyClient.Timeout = 30 * time.Minute
	}
	// Only the standard one-shot transport is supported; no caller retry wrapper.
	transport := client.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	standard, ok := transport.(*http.Transport)
	if !ok {
		return nil, ErrRuntimeTransport
	}
	copyClient.Transport = standard.Clone()
	copyClient.Transport.(*http.Transport).DisableKeepAlives = true

	if !target.Edge {
		u, err := url.Parse(target.Endpoint)
		if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") || (u.Path != "" && u.Path != "/") {
			return nil, ErrRuntimeTransport
		}
	}
	return &RuntimeSender{target: target, access: access, client: &copyClient, tunnels: tunnels}, nil
}
func (s *RuntimeSender) Execute(ctx context.Context, envelope activitytypes.RuntimeExecutionEnvelope) (activitytypes.RuntimeExecutionOutcome, error) {
	body, err := json.Marshal(envelope, json.FormatNilSliceAsNull(true))
	if err != nil {
		return activitytypes.RuntimeExecutionOutcome{}, ErrRuntimeTransport
	}
	return s.request(ctx, http.MethodPost, "/api/environments/0/runtime-runs", "", body, envelope.Identity, true)
}
func (s *RuntimeSender) Query(ctx context.Context, id activitytypes.RuntimeRunIdentity) (activitytypes.RuntimeExecutionOutcome, error) {
	return s.request(ctx, http.MethodGet, "/api/environments/0/runtime-runs/"+url.PathEscape(id.RunID), activitytypes.RuntimeIdentityQuery(id), nil, id, false)
}
func (s *RuntimeSender) Cancel(ctx context.Context, cancel activitytypes.RuntimeCancellation) (activitytypes.RuntimeExecutionOutcome, error) {
	body, err := json.Marshal(cancel)
	if err != nil {
		return activitytypes.RuntimeExecutionOutcome{}, ErrRuntimeTransport
	}
	return s.request(ctx, http.MethodPost, "/api/environments/0/runtime-runs/"+url.PathEscape(cancel.Identity.RunID)+"/cancel", "", body, cancel.Identity, false)
}
func (s *RuntimeSender) request(ctx context.Context, method, path, query string, body []byte, id activitytypes.RuntimeRunIdentity, execute bool) (activitytypes.RuntimeExecutionOutcome, error) {
	empty := activitytypes.RuntimeExecutionOutcome{}
	if s == nil || !activitytypes.RuntimeIdentityValid(id) || len(body) > activitytypes.RuntimeMaxBodyBytes {
		return empty, ErrRuntimeTransport
	}
	status, response, err := s.roundTrip(ctx, method, path, query, body)
	if err != nil {
		return empty, err
	}
	if execute && status == 409 {
		var refusal activitytypes.RuntimeExecutionRefusal
		if activitytypes.DecodeRuntimeJSON(response, &refusal) == nil && refusal.Safe() {
			return empty, &RuntimeExecutionRefused{Code: refusal.Code}
		}
	}
	if status != 200 {
		return empty, ErrRuntimeTransport
	}
	var result activitytypes.RuntimeExecutionOutcome
	if activitytypes.DecodeRuntimeJSON(response, &result) != nil || activitytypes.ValidateRuntimeOutcome(result, id) != nil {
		return empty, ErrRuntimeTransport
	}
	return result, nil
}

// Metadata requests use the identical authenticated one-shot transport and Edge
// ingress bounds, without granting an Execute refusal any broader authority.
func (s *RuntimeSender) roundTrip(ctx context.Context, method, path, query string, body []byte) (int, []byte, error) {
	if s == nil || len(body) > activitytypes.RuntimeMaxBodyBytes {
		return 0, nil, ErrRuntimeTransport
	}
	ctx, cancelBudget := context.WithTimeout(ctx, 30*time.Minute)
	defer cancelBudget()
	accessCtx, cleanup, err := s.access(ctx, s.target.EnvironmentID, &s.target.Token, method, path)
	if err != nil {
		return 0, nil, err
	}
	if cleanup == nil || accessCtx == nil {
		return 0, nil, ErrRuntimeTransport
	}
	defer cleanup()
	if accessCtx.Err() != nil {
		return 0, nil, ErrRuntimeTransport
	}
	headers := map[string]string{"Content-Type": "application/json", "Accept": "application/json", remenv.HeaderAgentToken: s.target.Token}
	var status int
	var contentType string
	var response []byte
	if s.target.Edge {
		if s.tunnels == nil {
			return 0, nil, ErrRuntimeTransport
		}
		tunnel, ok := s.tunnels.Get(s.target.EnvironmentID).Get()
		if !ok || edge.RequireSessionKey(accessCtx, tunnel) != nil {
			return 0, nil, ErrRuntimeTransport
		}
		command, ok := edge.RuntimeCommandName(method, path)
		if !ok {
			return 0, nil, ErrRuntimeTransport
		}
		result, err := edge.NewCommandClient().Execute(accessCtx, tunnel, &edge.CommandRequest{Command: command, Method: method, Path: path, Query: query, Headers: headers, Body: body})
		if err != nil || result == nil {
			return 0, nil, ErrRuntimeTransport
		}
		status, response = result.Status, result.Body
		for key, value := range result.Headers {
			if strings.EqualFold(key, "Content-Type") {
				contentType = value
			}
		}
	} else {
		endpoint := s.target.Endpoint
		if endpoint[len(endpoint)-1:] == "/" {
			endpoint = endpoint[:len(endpoint)-1]
		}
		endpoint += path
		if query != "" {
			endpoint += "?" + query
		}
		var reader io.Reader
		if len(body) > 0 {
			reader = io.NopCloser(bytes.NewReader(body))
		}
		req, err := http.NewRequestWithContext(accessCtx, method, endpoint, reader)
		if err != nil {
			return 0, nil, ErrRuntimeTransport
		}
		req.GetBody = nil
		req.ContentLength = int64(len(body))
		for key, value := range headers {
			req.Header.Set(key, value)
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return 0, nil, ErrRuntimeTransport
		}
		defer resp.Body.Close()
		response, err = io.ReadAll(io.LimitReader(resp.Body, activitytypes.RuntimeMaxBodyBytes+1))
		if err != nil {
			return 0, nil, ErrRuntimeTransport
		}
		status = resp.StatusCode
		contentType = resp.Header.Get("Content-Type")
	}
	media, _, mediaErr := mime.ParseMediaType(contentType)
	if mediaErr != nil || media != "application/json" {
		return 0, nil, ErrRuntimeTransport
	}
	if len(response) > activitytypes.RuntimeMaxBodyBytes {
		return 0, nil, ErrRuntimeTransport
	}
	return status, response, nil
}

func (s *RuntimeSender) Register(ctx context.Context, registration activitytypes.RuntimeRegistration) (activitytypes.RuntimeExecutorIdentity, error) {
	var result activitytypes.RuntimeExecutorIdentity
	err := s.metadata(ctx, "/api/environments/0/runtime-registration", registration, &result)
	if err == nil && (result.ManagerID != registration.ManagerID || result.RegistrationID != registration.RegistrationID || result.ExecutorGeneration == "") {
		err = ErrRuntimeTransport
	}
	return result, err
}
func (s *RuntimeSender) Resolve(ctx context.Context, request activitytypes.RuntimeTargetRequest) (activitytypes.RuntimeTargetMetadata, error) {
	var result activitytypes.RuntimeTargetMetadata
	err := s.metadata(ctx, "/api/environments/0/runtime-target", request, &result)
	if err == nil && (result.Target.Kind != request.Target.Kind || !runtimeTargetValidInternal(result.Target) ||
		(result.Target.ID != request.Target.ID && (request.Target.Kind != "container" || runtimeFullContainerID(request.Target.ID) || !runtimeFullContainerID(result.Target.ID)))) {
		err = ErrRuntimeTransport
	}
	return result, err
}
func (s *RuntimeSender) metadata(ctx context.Context, path string, value, result any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return ErrRuntimeTransport
	}
	status, response, err := s.roundTrip(ctx, http.MethodPost, path, "", body)
	if err != nil {
		return err
	}
	if status != 200 {
		var problem imageDTO.OperationError
		if status >= 400 && status <= 599 && activitytypes.DecodeRuntimeJSON(response, &problem) == nil && !problem.Success {
			field, _ := reflect.TypeFor[imageDTO.Problem]().FieldByName("Code")
			if slices.Contains(strings.Split(field.Tag.Get("enum"), ","), problem.Problem.Code) {
				return imageDTO.Failure(problem.Problem.Code, status)
			}
		}
		return ErrRuntimeTransport
	}
	if activitytypes.DecodeRuntimeJSON(response, result) != nil {
		return ErrRuntimeTransport
	}
	return nil
}
