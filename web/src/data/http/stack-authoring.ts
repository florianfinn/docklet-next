// Source: docklet-next/docs/design/stack-authoring.md
// Revision: e1fda6eb0c13b6b5cd95a59f65836bf68339de61
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original stack authoring mapping for issues #138 and #139.

import type { DeployInput, DockerRunConversion, DockletAdapter } from "../adapter";
import { StackCreatedError } from "../adapter";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "../capability";
import { normalizeStackLabels } from "@/domain/stack-labels";
import type { CsrfClient } from "./csrf-client";
import { inventoryId, splitInventoryId } from "./inventory-mapping";
import { object, runtimeResponseError } from "./runtime-mapping";

const failure = (code: string, status?: number) => new AdapterOutcomeError({ kind: "error", code, status });

interface Options {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  /** Guarded M6 `up` admission; returns the run id. */
  up: (stackId: string) => Promise<string>;
  /** Current server labels of a stack, read before a diff. */
  labels: (stackId: string) => Promise<string[]>;
  changed: () => void;
  unauthorized: () => void;
}

type Authoring = Pick<DockletAdapter, "deployStack" | "saveStackFiles" | "setStackLabels" | "convertDockerRun">;

/** Writes go through existing project routes; deploy reuses the M6 runtime admission. */
export function createStackAuthoring({ baseUrl, client, capability, up, labels, changed, unauthorized }: Options): Authoring {
  async function require(operation: string, ref: ResourceRef) {
    const descriptor = await capability(operation, ref);
    if (!descriptor.available) throw unavailable(operation, descriptor.resourceRef, descriptor.disabledReasonCode);
  }
  /** One request without retry; a send failure after dispatch is reported as unknown. */
  async function send(path: string, init: RequestInit, unknownCode: string, envelope = true): Promise<Record<string, unknown>> {
    let response: Response;
    try {
      response = await client.fetch(`${baseUrl}${path}`, { credentials: "same-origin", cache: "no-store", ...init, headers: { Accept: "application/json", ...init.headers } });
    } catch (error) {
      if (error instanceof AdapterOutcomeError) {
        if (error.outcome.kind === "error" && error.outcome.status === 401) unauthorized();
        throw error;
      }
      throw failure(unknownCode);
    }
    let body: unknown;
    try { body = await response.json(); } catch { throw failure(response.ok ? unknownCode : "http.error", response.status); }
    if (response.status === 401) unauthorized();
    if (!response.ok) throw runtimeResponseError(body, response.status);
    if (object(body).success !== true) throw failure(unknownCode, response.status);
    return envelope ? object(object(body).data) : object(body);
  }
  const json = (method: string, value: object): RequestInit => ({ method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(value) });
  const projectPath = (host: string, project: string) => `/environments/${encodeURIComponent(host)}/projects/${encodeURIComponent(project)}`;

  async function save(stackId: string, compose: string, env: string) {
    const [host, project] = splitInventoryId(stackId);
    await send(projectPath(host, project), json("PUT", { composeContent: compose, envContent: env }), "stack.save-unknown");
  }
  async function relabel(stackId: string, next: string[]) {
    const [host, project] = splitInventoryId(stackId);
    const current = normalizeStackLabels(await labels(stackId));
    const wanted = normalizeStackLabels(next);
    const changes = [...wanted.filter((name) => !current.includes(name)).map((name) => ({ name, attached: true })),
      ...current.filter((name) => !wanted.includes(name)).map((name) => ({ name, attached: false }))];
    for (const change of changes) await send(`${projectPath(host, project)}/tags`, json("PATCH", change), "stack.labels-unknown");
  }

  return {
    async deployStack(input: DeployInput) {
      if (input.stackId) {
        const [host] = splitInventoryId(input.stackId);
        if (host !== input.hostId) throw failure("stack.host-immutable");
        await require("deployStack", { kind: "stack", id: input.stackId });
        try {
          await save(input.stackId, input.compose, input.env);
          await relabel(input.stackId, input.labels);
        } finally { changed(); }
        return { actionId: await up(input.stackId), stackId: input.stackId };
      }
      await require("deployStack", { kind: "host", id: input.hostId });
      const form = new FormData();
      form.append("project", JSON.stringify({ name: input.name, composeContent: input.compose, envContent: input.env, tags: normalizeStackLabels(input.labels) }));
      form.append("manifest", JSON.stringify({ fileChanges: [] }));
      let created: Record<string, unknown>;
      try { created = await send(`/environments/${encodeURIComponent(input.hostId)}/projects`, { method: "POST", body: form }, "stack.create-unknown"); }
      finally { changed(); }
      if (typeof created.id !== "string" || !created.id || created.id.includes("~")) throw failure("stack.create-unknown");
      const stackId = inventoryId(input.hostId, created.id);
      try { return { actionId: await up(stackId), stackId }; }
      catch (error) { throw new StackCreatedError(stackId, error); }
    },
    async saveStackFiles(stackId, compose, env) {
      await require("saveStackFiles", { kind: "stack", id: stackId });
      try { await save(stackId, compose, env); } finally { changed(); }
    },
    async setStackLabels(stackId, next) {
      await require("setStackLabels", { kind: "stack", id: stackId });
      try { await relabel(stackId, next); } finally { changed(); }
    },
    async convertDockerRun(hostId, command): Promise<DockerRunConversion> {
      await require("convertDockerRun", { kind: "host", id: hostId });
      let data: Record<string, unknown>;
      try { data = await send(`/environments/${encodeURIComponent(hostId)}/system/convert`, json("POST", { dockerRunCommand: command }), "stack.convert-failed", false); }
      catch (error) {
        // The hub answers 400 for syntax it cannot parse; other failures stay transport errors.
        if (error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.status === 400) throw failure("stack.convert-invalid", 400);
        throw error;
      }
      if (typeof data.dockerCompose !== "string" || typeof data.serviceName !== "string") throw failure("stack.convert-failed");
      return { compose: data.dockerCompose, env: typeof data.envVars === "string" ? data.envVars : "", serviceName: data.serviceName };
    },
  };
}
