// Source: docklet-next/issues/586
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original setup test with snapshot-owned provider cleanup.

package tailnet

import (
	"context"
	"errors"
	"net/http"
	"slices"

	dto "github.com/florianfinn/docklet-next/types/tailnet"
)

// Test runs once per hub. Cleanup belongs to the starting credential even after reset or cancellation.
func (m *Manager) Test(ctx context.Context) (dto.TestResult, error) {
	if !m.testMu.TryLock() {
		return dto.TestResult{}, &dto.Problem{Code: dto.CodeTestRunning, Status: http.StatusConflict}
	}
	defer m.testMu.Unlock()
	m.mu.Lock()
	// The provisioning sequence is reserved with the snapshot: a credential save after this point fences it.
	cred, revision, gen := m.cred, m.credentialRevisionLocked(), m.gen
	m.provSeq++
	seq := m.provSeq
	m.mu.Unlock()
	out := dto.TestResult{CredentialRevision: revision, Checks: []dto.TestCheck{
		{ID: "credential", State: "skipped"}, {ID: "agent-key", State: "skipped"}, {ID: "cleanup", State: "skipped"},
	}}
	if cred.Mode == dto.ModeOAuth {
		tokenCtx, cancel := context.WithTimeout(ctx, providerTimeout)
		token, err := m.cfg.Provisioner.AccessToken(tokenCtx, cred.ClientID, cred.ClientSecret)
		cancel()
		out.Checks[0] = providerCheck("credential", err, dto.CodeProviderUnavailable)
		if err == nil {
			mintCtx, stop := context.WithTimeout(ctx, providerTimeout)
			id, mintErr := m.cfg.Provisioner.MintTestKey(mintCtx, token)
			stop()
			// The test mints the same host key the route mints: its answer (ok or a definite refusal)
			// is the provisioning state for this credential, so a passing test clears an earlier refusal.
			m.recordProvisioning(gen, seq, cred, mintErr, false)
			out.Checks[1] = providerCheck("agent-key", mintErr, dto.CodeProviderUnavailable)
			switch {
			case mintErr != nil:
				if out.Checks[1].Code != dto.CodeMintRefused && out.Checks[1].Code != dto.CodeTagsNotPermitted {
					out.Checks[2].State = "unknown"
				}
			case id == "":
				out.Checks[2] = dto.TestCheck{ID: "cleanup", State: "fail", Code: dto.CodeTestCleanupFailed}
			default:
				cleanupCtx, done := context.WithTimeout(context.WithoutCancel(ctx), providerTimeout)
				cleanupErr := m.cfg.Provisioner.DeleteKey(cleanupCtx, token, id)
				done()
				out.Checks[2] = providerCheck("cleanup", cleanupErr, dto.CodeTestCleanupFailed)
			}
		}
	}
	current := m.RetryJoin(ctx, revision)
	// Membership, tags, address and credential identity come from one completion snapshot.
	m.mu.Lock()
	defer m.mu.Unlock()
	out.Stale = !current || revision != m.credentialRevisionLocked()
	joined := m.conn.State == "connected"
	hub := []dto.TestCheck{{ID: "hub-joined", State: "skipped"}, {ID: "hub-tag", State: "skipped"}, {ID: "hub-address", State: "skipped"}}
	if !out.Stale {
		hub[0].State = "fail"
		hub[0].Code = dto.CodeNotConnected
		if m.conn.Code == dto.CodeCredentialRejected || m.conn.Code == dto.CodeTagsNotPermitted {
			hub[0].Code, hub[0].RefusedTags = m.conn.Code, m.conn.RefusedTags
		}
		if joined {
			hub[0] = dto.TestCheck{ID: "hub-joined", State: "pass"}
			hub[2] = dto.TestCheck{ID: "hub-address", State: "fail"}
			if m.status != nil {
				if hubIPv4(m.status) != "" {
					hub[2] = dto.TestCheck{ID: "hub-address", State: "pass"}
				}
				if m.status.Self != nil {
					hub[1] = dto.TestCheck{ID: "hub-tag", State: "fail"}
					if m.status.Self.Tags != nil && slices.Contains(m.status.Self.Tags.AsSlice(), HubTag) {
						hub[1] = dto.TestCheck{ID: "hub-tag", State: "pass"}
					}
				}
			}
		}
	}
	out.Checks = append(out.Checks, hub...)
	return out, nil
}

func providerCheck(id string, err error, fallback string) dto.TestCheck {
	row := dto.TestCheck{ID: id, State: "pass"}
	if err == nil {
		return row
	}
	row.State, row.Code = "fail", fallback
	var problem *dto.Problem
	if errors.As(err, &problem) {
		row.ProviderStatus = problem.ProviderStatus
		if problem.Code == dto.CodeTagsNotPermitted || id == "credential" && problem.Code == dto.CodeCredentialRejected || id == "agent-key" && problem.Code == dto.CodeMintRefused {
			row.Code, row.RefusedTags = problem.Code, problem.RefusedTags
		}
	}
	return row
}
