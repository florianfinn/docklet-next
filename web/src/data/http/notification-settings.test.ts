// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original producer-fixture and transport proofs for notification targets and Auto Heal.

import { describe, expect, it, vi } from "vitest";

import { AdapterOutcomeError } from "../capability";
import { SettingsProblemError } from "../settings-error";
import { EVENT_KEYS, parseNotificationTarget } from "@/domain/notifications";
import type { CsrfClient } from "./csrf-client";
import { createNotificationSettings } from "./notification-settings";
import fixture from "./testdata/notification-targets.json";

type Reply = { status?: number; body: unknown };

function api(reply: (url: string, init: RequestInit) => Reply) {
  const calls: { url: string; init: RequestInit }[] = [];
  const fetch = vi.fn(async (url: string, init: RequestInit = {}) => {
    calls.push({ url, init });
    const { status = 200, body } = reply(url, init);
    return new Response(JSON.stringify(body), { status });
  });
  const adapter = createNotificationSettings({
    baseUrl: "/api", client: { fetch } as unknown as CsrfClient, generation: () => 0,
    capability: async (operation, resourceRef) => ({ operationId: operation, resourceRef, available: true }),
  });
  return { adapter, calls };
}

const heal = { enabled: true, maxRestarts: 3, windowMinutes: 90, excludedStacks: ["web"], excludedContainers: null, managedByEnvironment: false };

describe("notification target transport", () => {
  it("reads the real producer list as typed targets", async () => {
    const { adapter } = api(() => ({ body: fixture }));
    const rows = await adapter.listNotificationTargets();
    expect(rows.map((r) => [r.kind, r.provider])).toEqual([["target", "email"], ["target", "ntfy"], ["target", "generic"]]);
    const [email, ntfy, generic] = rows;
    if (email?.kind !== "target" || ntfy?.kind !== "target" || generic?.kind !== "target") throw new Error("typed rows expected");
    expect(email.credentialsSet).toEqual(["smtpPassword"]);
    expect(email.config.smtpPassword).toBe("");
    // A missing wire key reads as enabled; updates stays on while container_update is absent.
    expect(email.events).toEqual(["deployFailed", "hostOffline", "backupFailed", "autoHeal", "updates", "security", "cleanup"]);
    expect(ntfy.enabled).toBe(false);
    expect(ntfy.config).not.toHaveProperty("port");
    expect(generic.events).not.toContain("security");
  });

  it("shows rows outside the typed contract as unsupported instead of editing them", () => {
    const email = fixture[0]!;
    const cases: Record<string, unknown> = {
      provider: { ...email, provider: "pager" },
      key: { ...email, config: { ...email.config, unexpected: "x" } },
      kind: { ...email, config: { ...email.config, smtpPort: "587" } },
      event: { ...email, config: { ...email.config, events: { deploy_failed: true, unknown_event: true } } },
      eventValue: { ...email, config: { ...email.config, events: { deploy_failed: "yes" } } },
      list: { ...email, config: { ...email.config, toAddresses: [1] } },
      header: { ...fixture[2]!, config: { ...fixture[2]!.config, customHeaders: { "X-Example": 1 } } },
    };
    for (const [name, row] of Object.entries(cases)) expect([name, parseNotificationTarget(row)?.kind]).toEqual([name, "unsupported"]);
    expect(parseNotificationTarget({ ...email, id: "1" })).toBeUndefined();
  });

  it("refuses a list row without an identity", async () => {
    const { adapter } = api(() => ({ body: [{ provider: "email" }] }));
    await expect(adapter.listNotificationTargets()).rejects.toBeInstanceOf(AdapterOutcomeError);
  });

  it("saves an explicit map of every wire event and maps a field refusal", async () => {
    let refuse = false;
    const { adapter, calls } = api(() => (refuse
      ? { status: 400, body: { code: "notification.config-invalid", field: "/config/smtpHost", detail: "server text" } }
      : { body: fixture[0] }));
    const email = fixture[0]!;
    await adapter.saveNotificationTarget({ provider: "email", enabled: true, config: { ...email.config, events: undefined }, events: ["deployFailed", "updates"] });
    const sent = JSON.parse(calls[0]!.init.body as string) as { config: { events: Record<string, boolean> } };
    const keys = Object.values(EVENT_KEYS).flat();
    expect(Object.keys(sent.config.events).sort()).toEqual([...keys].sort());
    expect(Object.entries(sent.config.events).filter(([, on]) => on).map(([k]) => k).sort()).toEqual(["container_update", "deploy_failed", "image_update"]);
    refuse = true;
    const error = await adapter.saveNotificationTarget({ provider: "email", enabled: true, config: email.config, events: [] }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(SettingsProblemError);
    expect((error as SettingsProblemError).field).toBe("/config/smtpHost");
  });
});

describe("Auto Heal transport", () => {
  it("reads a host row and marks the hub row locked while UI configuration is disabled", async () => {
    let locked = "false";
    const { adapter } = api((url) => (url.endsWith("/settings") ? { body: [{ key: "uiConfigDisabled", value: locked, type: "boolean" }] } : { body: heal }));
    expect(await adapter.getAutoHealSettings("agent-1")).toEqual({ ...heal, excludedContainers: [] });
    expect((await adapter.getAutoHealSettings("0")).configurationLocked).toBeUndefined();
    locked = "true";
    expect((await adapter.getAutoHealSettings("0")).configurationLocked).toBe(true);
  });

  it("keeps the hub row editable when the lock read fails and lets the save refuse", async () => {
    const { adapter } = api((url, init) => (url.endsWith("/settings") ? { status: 500, body: {} } : init.method === "PUT" ? { status: 403, body: { title: "Forbidden", detail: "server text" } } : { body: heal }));
    expect((await adapter.getAutoHealSettings("0")).configurationLocked).toBeUndefined();
    const error = await adapter.saveAutoHealSettings("0", { enabled: false, maxRestarts: 3, windowMinutes: 90, excludedStacks: [], excludedContainers: [] }).catch((e: unknown) => e);
    expect(error).toBeInstanceOf(AdapterOutcomeError);
    expect(error).not.toBeInstanceOf(SettingsProblemError);
    expect((error as AdapterOutcomeError).outcome).toEqual({ kind: "error", code: "http.error", status: 403 });
  });

  it("refuses a malformed producer row", async () => {
    for (const body of [{ ...heal, maxRestarts: 0 }, { ...heal, windowMinutes: 4 }, { ...heal, managedByEnvironment: "no" }, { ...heal, excludedStacks: [1] }]) {
      const { adapter } = api(() => ({ body }));
      await expect(adapter.getAutoHealSettings("agent-1")).rejects.toBeInstanceOf(AdapterOutcomeError);
    }
  });

  it("maps an environment-managed refusal to its typed code", async () => {
    const { adapter } = api(() => ({ status: 409, body: { code: "autoheal.settings-env-managed" } }));
    const error = await adapter.saveAutoHealSettings("agent-1", { enabled: true, maxRestarts: 3, windowMinutes: 60, excludedStacks: [], excludedContainers: [] }).catch((e: unknown) => e);
    expect((error as SettingsProblemError).code).toBe("autoheal.settings-env-managed");
  });
});
