// Source: docklet-next/issues/583
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity titles, host targets and scan presentation. M37 #584: truthful activity titles, host targets and scan counts.

import { CancelledError, isCancelledError } from "@tanstack/react-query";
import type { CapabilityDescriptor } from "@/data/capability";
import { unavailable } from "@/data/capability";
import type { CodedActionRun } from "@/data/message-code";
import { readCandidateRun, readScanProgress } from "@/data/http/vulnerabilities";
import type { vulnerabilityTransport } from "@/data/http/vulnerability-transport";

interface Options {
  request: ReturnType<typeof vulnerabilityTransport>;
  host: string;
  hostScan: boolean;
  id: string;
  signal?: AbortSignal;
  capability: () => Promise<CapabilityDescriptor>;
  hosts: () => Promise<unknown>;
  registration: () => string | undefined;
}

/** Durable counts belong to this detail's host, activity and registration lifetime. */
export async function readActivityScanCounts(run: CodedActionRun, o: Options): Promise<void> {
  const manual = run.kind === "scan" && o.hostScan;
  const candidate = run.sourceOperation === "vulnerability.candidate-scan" && !["running", "queued"].includes(run.status);
  if (!manual && !candidate) return;
  try {
    if (o.registration() === undefined) await o.hosts();
    const registration = o.registration(), capability = await o.capability();
    if (!capability.available) throw unavailable("getSecurity", capability.resourceRef, capability.disabledReasonCode);
    const counts = manual ? await readScanProgress(o.request, o.host, o.id, o.signal) : await readCandidateRun(o.request, o.host, o.id, o.signal);
    if (registration !== o.registration()) throw new CancelledError();
    if (counts && "done" in counts) {
      run.scan = counts;
      run.scanProgress = counts.targets > 0 ? Math.min(100, Math.round(100 * counts.done / counts.targets)) : undefined;
    } else if (counts && counts.status !== "running") run.candidateScan = counts;
  } catch (error) { if (isCancelledError(error)) throw error; }
}
