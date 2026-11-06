// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original demo policy and baseline reads matching bulk update admission. M19 web consumer integration.

import type { UpdatesApi } from "@/domain/stack-updates";
import { AdapterOutcomeError } from "../capability";
import type { DemoContext } from "./demo-context";
export function createDemoBulkUpdates({ db }: DemoContext): Pick<UpdatesApi, "getAutoUpdatePolicy" | "getUpdateBaseline"> {
  const stack = (id: string) => { const value = db.stack(id); if (!value) throw new AdapterOutcomeError({ kind: "error", code: "runtime.target-unavailable" }); return value; };
  return {
    async getAutoUpdatePolicy(id) {
      const value = stack(id);
      return { mode: "check", revision: 1, environmentId: value.hostId, registrationId: value.hostId, targetKind: "stack", targetName: value.name, targetMissing: false, targetId: id, targetIdentity: id, jobId: id, stale: false,
        candidates: value.services.flatMap((service) => service.update.state === "version" || service.update.state === "rebuild" ? [{ service: service.name, imageRef: `${service.image.repository}:${service.image.tag}`, version: service.update.state === "version" ? service.update.latest.version ?? service.update.latest.tag : service.image.tag, digest: "sha256:" + "a".repeat(64), platform: "linux/amd64", notificationState: "not-requested" as const }] : []) };
    },
    async getUpdateBaseline(id) { stack(id); return { targetIdentity: id, savedRevision: "a".repeat(64), observedRevision: "a".repeat(64), available: true, autoExcluded: false }; },
  };
}
