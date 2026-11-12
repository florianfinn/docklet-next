// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { CancelledError, isCancelledError } from "@tanstack/react-query";
import type { components } from "@/api/generated/schema";
import type { DockletAdapter } from "@/data/adapter";
import { AdapterOutcomeError, unavailable, type CapabilityDescriptor, type ResourceRef } from "@/data/capability";
import type { ImagePruneHostReport } from "@/domain/images";
import type { CsrfClient } from "./csrf-client";
import { discardResponse, releaseResponse } from "./identity-reads";
import { splitInventoryId, inventoryId } from "./inventory-mapping";
import { mapPullProgress } from "./image-mapping";
import { imageErrorCode, imageResponseError, imageUnknown, record } from "./image-errors";

type Schema = components["schemas"];
interface Options {
  baseUrl: string;
  client: CsrfClient;
  capability: (operation: string, ref: ResourceRef) => Promise<CapabilityDescriptor>;
  generation: () => number;
  changed: () => void;
}
export function createImageCommands({ baseUrl, client, capability, generation, changed }: Options): Pick<DockletAdapter, "pullImage" | "removeImage" | "pruneImages"> {
  const check = (owner: number) => { if (owner !== generation()) throw new CancelledError(); };
  async function require(operation: string, ref: ResourceRef, owner: number) {
    const result = await capability(operation, ref); check(owner);
    if (!result.available) throw unavailable(operation, ref, result.disabledReasonCode);
  }
  async function request(host: string, suffix: string, method: string, body: object | undefined, owner: number) {
    check(owner);
    try {
      const response = await client.fetch(`${baseUrl}/environments/${encodeURIComponent(host)}/images${suffix}`, { method, credentials: "same-origin", cache: "no-store", headers: { Accept: "application/json, application/x-json-stream", ...(body ? { "Content-Type": "application/json" } : {}) }, ...(body ? { body: JSON.stringify(body) } : {}) });
      check(owner);
      if (!response.ok) {
        const value: unknown = await response.json(); check(owner);
        throw imageResponseError(value, response.status);
      }
      return response;
    } catch (error) { check(owner); if (isCancelledError(error) || error instanceof AdapterOutcomeError) throw error; throw imageUnknown(); }
  }
  async function json(response: Response, owner: number): Promise<unknown> {
    let body: unknown;
    try { body = await response.json(); } catch { check(owner); throw imageUnknown(); }
    check(owner);
    if (record(body).success !== true || !("data" in record(body))) throw imageUnknown();
    return record(body).data;
  }
  return {
    async pullImage(input) {
      const owner = generation();
      await require("pullImage", { kind: "host", id: input.hostId }, owner);
      const body: Schema["ImagePullOptions"] = { imageName: input.reference.trim() };
      let actionId: string | undefined;
      try {
        const response = await request(input.hostId, "/pull", "POST", body, owner);
        const reader = response.body?.getReader();
        if (!reader) throw imageUnknown();
        const decoder = new TextDecoder(); let pending = "", done = false, ended = false;
        function frame(line: string) {
          if (!line.trim()) return;
          const value: unknown = JSON.parse(line); const row = record(value);
          if (done) throw imageUnknown();
          if ("error" in row) throw imageResponseError(value);
          if (row.done === true) { const terminal: Schema["ImagePullDoneFrame"] = { done: true }; done = terminal.done; }
          else if (row.type === "activity" && typeof row.activityId === "string" && row.activityId) { const started: Schema["ImagePullStartedFrame"] = { type: "activity", activityId: row.activityId }; actionId = started.activityId; }
          else {
            const progress = mapPullProgress(row);
            if (!progress) throw imageUnknown();
            input.onProgress?.(progress);
          }
        }
        try {
          for (;;) {
            const chunk = await reader.read(); check(owner);
            pending += decoder.decode(chunk.value, { stream: !chunk.done });
            let newline: number;
            while ((newline = pending.indexOf("\n")) >= 0) { frame(pending.slice(0, newline)); pending = pending.slice(newline + 1); }
            // Complete frames may arrive together in a large network chunk. Bound only the unfinished line.
            if (pending.length > 65536) throw imageUnknown();
            if (chunk.done) { ended = true; break; }
          }
          if (pending.trim()) frame(pending);
          if (!done) throw imageUnknown();
        } finally {
          // A stream read to its end is released, not cancelled: cancelling it reports the finished pull as aborted.
          if (ended) { reader.releaseLock(); releaseResponse(response); }
          else { await reader.cancel().catch(() => {}); reader.releaseLock(); discardResponse(response); }
        }
        return { actionId };
      } catch (error) {
        check(owner);
        if (isCancelledError(error)) throw error;
        const failure = error instanceof AdapterOutcomeError ? error : imageUnknown();
        if (actionId) Object.assign(failure, { actionId });
        throw failure;
      } finally { if (owner === generation()) changed(); }
    },
    async removeImage(id, force = false) {
      const owner = generation(), [host, raw] = splitInventoryId(id);
      await require("removeImage", { kind: "image", id }, owner);
      try { await json(await request(host, `/${encodeURIComponent(raw)}?force=${force}`, "DELETE", undefined, owner), owner); }
      finally { if (owner === generation()) changed(); }
    },
    async pruneImages({ hostIds, mode }) {
      const owner = generation(), unique = [...new Set(hostIds)];
      // Check every scope before the first write; a hub aggregate never authorizes a target.
      await Promise.all(unique.map((id) => require("pruneImages", { kind: "host", id }, owner)));
      const hosts = await Promise.all(unique.map(async (hostId): Promise<ImagePruneHostReport> => {
        try {
          const body: Schema["PruneImagesInputBody"] = { mode: mode === "unused" ? "all" : "dangling" };
          const value = await json(await request(hostId, "/prune", "POST", body, owner), owner);
          const row = record(value) as Partial<Schema["ImagePruneReport"]>;
          if (![row.imagesDeleted, row.skipped, row.failed].every((items) => items === null || Array.isArray(items)) || typeof row.spaceReclaimedKnown !== "boolean" || typeof row.spaceReclaimed !== "number") throw imageUnknown();
          const removed = (row.imagesDeleted ?? []).map((id) => { if (typeof id !== "string") throw imageUnknown(); return inventoryId(hostId, id); });
          const items = (entries: Schema["ImagePruneItem"][] | null | undefined) => (entries ?? []).map((entry) => {
            if (typeof entry?.id !== "string" || typeof entry.code !== "string") throw imageUnknown();
            return { id: inventoryId(hostId, entry.id), code: entry.code };
          });
          return { hostId, removed, skipped: items(row.skipped), failed: items(row.failed), ...(row.spaceReclaimedKnown ? { spaceReclaimed: row.spaceReclaimed } : {}) };
        } catch (error) {
          check(owner); if (isCancelledError(error)) throw error;
          const code = record(record(error).outcome).code;
          return { hostId, removed: [], skipped: [], failed: [], errorCode: typeof code === "string" ? code : imageErrorCode(error) ?? "image.outcome-unknown" };
        } finally { if (owner === generation()) changed(); }
      }));
      return { hosts };
    },
  };
}
