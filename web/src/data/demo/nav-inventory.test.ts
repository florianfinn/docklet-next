// Source: docklet-next/issues/462
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original demo count parity and raw bind-source projection coverage.

import { expect, it } from "vitest";
import { createDemoAdapter } from "./demo-adapter";
import { DemoDb } from "./demo-db";
import { demoInventoryContainers, demoMountSources } from "./demo-nav-inventory";
import { createDisconnectedAdapter } from "../disconnected-adapter";

it("matches the demo Volumes page row count per host and exposes raw bind sources on container reads", async () => {
  const adapter = createDemoAdapter();
  const volumes = await adapter.listVolumes(), batches = await adapter.listNavInventory();
  for (const host of await adapter.listHosts()) expect(batches.find((h) => h.hostId === host.id)?.volumeTotal).toBe(volumes.filter((v) => v.hostId === host.id).length);
  expect((await adapter.listContainers()).every((c) => "bindSources" in c)).toBe(true);
  expect((await adapter.listContainers()).every((c) => c.imageId === undefined)).toBe(true);
});

it("reads bind sources from source configuration including long-form colon-containing paths", () => {
  const db = new DemoDb(), stack = db.stacks[0]!, service = stack.services[0]!;
  db.compose[stack.id] = `services:\n  ${service.name}:\n    volumes:\n      - type: bind\n        source: /srv/example:data\n        target: /data\n      - type: volume\n        source: named\n        target: /named\n`;
  expect(demoMountSources(db.compose[stack.id]!, service.name, { hostId: stack.hostId, stackId: stack.id })).toEqual({ binds: ["/srv/example:data"], volumes: [`${stack.id}_named`] });
  expect(demoInventoryContainers(db).find((c) => c.stackId === stack.id && c.serviceName === service.name)?.bindSources).toEqual(["/srv/example:data"]);
  db.hosts = db.hosts.filter((host) => host.id !== stack.hostId);
  expect(demoInventoryContainers(db).filter((c) => c.hostId === stack.hostId)).toHaveLength(0);
});

it("keeps disconnected shell reads unavailable", async () => {
  await expect(createDisconnectedAdapter().listNavInventory()).rejects.toBeDefined();
});
