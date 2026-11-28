// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original disruption timeline and bounded combined log window for #693.

import type { HostContainerHistory, HistoryBucket } from "@/domain/history";
import type { ContainerInfo, Host } from "@/domain/model";
export interface Participant { hostId: string; host: string; name: string; id?: string; minutes: number }
export interface WindowSelection { since: string; until: string; participants: Participant[]; omitted: number }
export const disrupted = (b: HistoryBucket) => b.minutes.down + b.minutes.unhealthy > 0;
export function selectWindow(bucket: HistoryBucket, histories: HostContainerHistory[], hosts: Host[], containers: ContainerInfo[]): WindowSelection {
  const since = Date.parse(bucket.start) - 300_000, until = Date.parse(bucket.end) + 300_000;
  const participants = histories.flatMap((h) => h.containers.flatMap((lane) => {
    const minutes = lane.buckets.filter((b) => Date.parse(b.start) < until && Date.parse(b.end) > since).reduce((sum, b) => sum + b.minutes.down + b.minutes.unhealthy, 0);
    if (!minutes) return [];
    return [{ hostId: h.hostId, host: hosts.find((host) => host.id === h.hostId)?.name ?? h.hostId, name: lane.name, id: containers.find((c) => c.hostId === h.hostId && c.name === lane.name)?.id, minutes }];
  })).sort((a, b) => b.minutes - a.minutes || a.hostId.localeCompare(b.hostId) || a.name.localeCompare(b.name));
  return { since: new Date(since).toISOString(), until: new Date(until).toISOString(), participants: participants.slice(0, 20), omitted: Math.max(0, participants.length - 20) };
}
