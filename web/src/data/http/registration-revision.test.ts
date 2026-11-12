// Source: docklet-next/docs/design/backup-completion.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regression for the registration revision in the host identity key (#288).

import { expect, it } from "vitest";
import { resourceFixture } from "./resources.test-support";

const at = "2026-11-02T00:00:00Z";

it("treats a registration revision change as a new host identity and discards held reads", async () => {
  const f = resourceFixture();
  const topics: string[][] = [];
  const stop = f.adapter.subscribeChanges((changed) => topics.push([...changed]));
  await f.adapter.listHosts();
  const rows = (revision: number) => f.state.hostIds.map((id) => ({ id, name: id, apiUrl: "http://agent.example.com", enabled: true, status: f.state.status, isEdge: f.state.isEdge, registrationRevision: revision }));
  const emit = (revision: number) => f.source.onmessage?.({ data: JSON.stringify({ timestamp: at, channel: "environments", environment: { environments: rows(revision) } }), lastEventId: "" } as MessageEvent);
  emit(1);
  emit(2);
  stop();
  // An unchanged row only refreshes the host list; a rotated registration also invalidates inventory and backup reads.
  expect([topics.at(-2)?.includes("volumes"), topics.at(-1)?.includes("volumes")]).toEqual([false, true]);
});
