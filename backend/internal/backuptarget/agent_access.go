// Source: docklet-next/issues/284
// Revision: e5218d798f5302f7fa568a6059a70901bcc83219
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original agent compatibility refusal and typed agent problems for central backup routes;
// the access envelope type is shared with the proxy translation (#292).

package backuptarget

import (
	"context"
	"encoding/json/v2"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"

	dto "github.com/florianfinn/docklet-next/types/backup"

	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// AgentAccessError is the shared host access envelope of proxied routes.
type AgentAccessError = handlerutil.AgentAccessError

// AgentAccess maps an agent access verdict (outdated agent, refused token or
// upstream 401/403) to its refusal and returns nil for every other error.
func AgentAccess(err error) *AgentAccessError { return handlerutil.AgentVerdict(err) }

// RemoteProblem rebuilds the typed backup refusal an agent answered with, so
// central routes keep its status, code and running activity. Only allowlisted
// codes and reasons pass; the public detail is derived from the code again.
func RemoteProblem(err error) *dto.Problem {
	var upstream *remenv.StatusError
	if !errors.As(err, &upstream) || upstream.StatusCode < 400 || upstream.StatusCode > 599 {
		return nil
	}
	var wire dto.Problem
	if json.Unmarshal(upstream.Body, &wire) != nil || wire.Status != upstream.StatusCode || !dto.IsProblemCode(wire.Code) {
		return nil
	}
	problem := dto.NewProblem(upstream.StatusCode, wire.Code, err)
	if _, parseErr := uuid.Parse(wire.ActivityID); parseErr == nil {
		problem.ActivityID = wire.ActivityID
	}
	if dto.IsProblemReason(wire.Reason) {
		problem.Reason = wire.Reason
	}
	problem.Volumes, problem.Hosts = remoteNamesInternal(wire.Volumes), remoteNamesInternal(wire.Hosts)
	return problem
}

// remoteNamesInternal passes an agent's volume or host name list only when it
// stays within display bounds; otherwise the refusal keeps its code without it.
func remoteNamesInternal(names []string) []string {
	if len(names) > remoteNamesMax {
		return nil
	}
	for _, name := range names {
		if name == "" || len(name) > remoteNameMaxBytes || !utf8.ValidString(name) || strings.ContainsFunc(name, func(r rune) bool { return !unicode.IsPrint(r) }) {
			return nil
		}
	}
	return names
}

const (
	remoteNamesMax     = 256
	remoteNameMaxBytes = 255
)

// RemoteJSON keeps the agent access verdict and typed backup refusals that
// the shared proxy translation reports as generic errors.
func RemoteJSON[T any](ctx context.Context, proxy handlerutil.RemoteJSONProxy, environmentID, method, path string, body any) (*T, error) {
	var raw error
	record := handlerutil.RemoteJSONProxy(func(ctx context.Context, id, m, p string, b []byte, out any) error {
		raw = proxy(ctx, id, m, p, b, out)
		return raw
	})
	output, err := record.JSON[T](ctx, environmentID, method, path, body)
	if err != nil && raw != nil {
		if access := AgentAccess(raw); access != nil {
			return nil, access
		}
		if problem := RemoteProblem(raw); problem != nil {
			return nil, problem
		}
	}
	return output, err
}
