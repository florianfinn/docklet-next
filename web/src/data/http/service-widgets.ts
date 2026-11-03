// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original service widget transport for issues #332 and #333.

import type { components } from "@/api/generated/schema";
import { isWidgetErrorCode, SERVICE_KINDS, type ServiceKind, type ServiceWidgetsApi, type WidgetConfig, type WidgetConfigInput, type WidgetError, type WidgetResult, type WidgetSummary } from "@/domain/service-widgets";
import { AdapterOutcomeError } from "../capability";
import type { CsrfClient } from "./csrf-client";
import { inventoryId, splitInventoryId } from "./inventory-mapping";

type Wire = components["schemas"];

function failure(body: unknown, status?: number): AdapterOutcomeError {
  const code = body && typeof body === "object" ? (body as { code?: unknown }).code : undefined;
  return new AdapterOutcomeError({ kind: "error", code: typeof code === "string" ? code : "http.error", status });
}

const invalid = () => failure({ code: "widget.bad-response" });
const isKind = (value: unknown): value is ServiceKind => (SERVICE_KINDS as readonly unknown[]).includes(value);
const stackId = (ref?: Wire["WidgetStackReference"]) => (ref?.environmentId && ref.projectId ? inventoryId(ref.environmentId, ref.projectId) : undefined);

function mapError(problem?: Wire["WidgetProblem"]): WidgetError | undefined {
  if (!problem) return undefined;
  // Codes from a newer hub render as the generic failure text.
  const code = isWidgetErrorCode(problem.code) ? problem.code : "widget.bad-response";
  return { code, ...(problem.certificate?.sha256 ? { certificate: { hostPort: problem.certificate.hostPort, sha256: problem.certificate.sha256 } } : {}) };
}

export function mapWidgetResult(wire: Wire["WidgetResult"]): WidgetResult {
  if (!wire || !isKind(wire.kind) || typeof wire.stale !== "boolean") throw invalid();
  const error = mapError(wire.error);
  return {
    kind: wire.kind, stale: wire.stale, ageSeconds: Math.max(0, wire.ageSeconds ?? 0),
    ...(wire.fetchedAt ? { fetchedAt: wire.fetchedAt } : {}),
    ...(error ? { error } : {}),
    ...(wire.plex ? { plex: { streams: wire.plex.streams, transcodes: wire.plex.transcodes, sessions: wire.plex.sessions ?? [] } } : {}),
    ...(wire.overseerr ? { overseerr: { pending: wire.overseerr.pending, approved: wire.overseerr.approved, available: wire.overseerr.available, requests: wire.overseerr.requests ?? [] } } : {}),
    ...(wire.minecraft ? { minecraft: { online: wire.minecraft.online, max: wire.minecraft.max, motd: wire.minecraft.motd, version: wire.minecraft.version, players: wire.minecraft.players ?? [] } } : {}),
  };
}

function mapConfig(wire: Wire["WidgetConfig"]): WidgetConfig {
  if (!wire || !isKind(wire.kind) || !Number.isInteger(wire.revision)) throw invalid();
  const stack = stackId(wire.stack);
  return {
    kind: wire.kind, enabled: wire.enabled, address: wire.address, hasCredential: wire.hasCredential, openUrl: wire.openUrl, revision: wire.revision,
    ...(stack ? { stackId: stack } : {}),
    ...(wire.tlsPin?.sha256 ? { tlsPin: { hostPort: wire.tlsPin.hostPort, sha256: wire.tlsPin.sha256 } } : {}),
  };
}

function wireInput(input: WidgetConfigInput): Wire["WidgetConfigUpdate"] {
  const [environmentId, projectId] = input.stackId ? splitInventoryId(input.stackId) : [];
  return {
    enabled: input.enabled, address: input.address.trim(), openUrl: input.openUrl.trim(), credentialAction: input.credentialAction,
    ...(input.credentialAction === "replace" ? { credential: input.credential ?? "" } : {}),
    ...(environmentId && projectId ? { stack: { environmentId, projectId } } : {}),
    ...(input.tlsPin ? { tlsPin: input.tlsPin } : {}),
  };
}

async function request<T>(client: CsrfClient, url: string, init: RequestInit = {}): Promise<T> {
  let response: Response;
  try {
    response = await client.fetch(url, { credentials: "same-origin", cache: "no-store", ...init, headers: { Accept: "application/json", ...(init.body ? { "Content-Type": "application/json" } : {}) } });
  } catch (error) { if (error instanceof DOMException && error.name === "AbortError") throw error; throw failure(null); }
  let body: unknown;
  try { body = await response.json(); } catch { throw failure(null, response.status); }
  if (!response.ok) throw failure(body, response.status);
  return body as T;
}

export function createHttpServiceWidgets(baseUrl: string, client: CsrfClient): ServiceWidgetsApi {
  const root = `${baseUrl}/service-widgets`;
  return {
    async listServiceWidgets() {
      const rows = await request<Wire["WidgetSummary"][] | null>(client, root);
      return (rows ?? []).filter((row) => isKind(row?.kind)).map((row): WidgetSummary => {
        const stack = stackId(row.stack);
        return { kind: row.kind, configured: row.configured, enabled: row.enabled, openUrl: row.openUrl, ...(stack ? { stackId: stack } : {}) };
      });
    },
    async getServiceWidget(kind) {
      return mapWidgetResult(await request(client, `${root}/${kind}`));
    },
    async getServiceWidgetConfig(kind) {
      return mapConfig(await request(client, `${root}/${kind}/config`));
    },
    async saveServiceWidgetConfig(kind, input) {
      return mapConfig(await request(client, `${root}/${kind}/config`, { method: "PUT", body: JSON.stringify(wireInput(input)) }));
    },
    async testServiceWidget(kind, input) {
      return mapWidgetResult(await request(client, `${root}/${kind}/test`, { method: "POST", body: JSON.stringify(wireInput(input)) }));
    },
    async decideMediaRequest(id, decision) {
      await request(client, `${root}/overseerr/requests/${encodeURIComponent(String(id))}/${decision}`, { method: "POST" });
    },
  };
}
