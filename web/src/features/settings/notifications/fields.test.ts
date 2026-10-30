// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original schema parity proof for the notification config tables.

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import { CONFIG_KEYS, EVENT_KEYS, NOTIFICATION_PROVIDERS } from "@/domain/notifications";

import { DEFAULTS, FIELDS, fromText, toText } from "./fields";

const SCHEMA: Record<string, string> = {
  email: "NotificationConfigEmail", discord: "NotificationConfigDiscord", telegram: "NotificationConfigTelegram", signal: "NotificationConfigSignal",
  slack: "NotificationConfigSlack", ntfy: "NotificationConfigNtfy", pushover: "NotificationConfigPushover", gotify: "NotificationConfigGotify",
  matrix: "NotificationConfigMatrix", googlechat: "NotificationConfigGoogleChat", generic: "NotificationConfigGeneric",
};

describe("notification config tables", () => {
  const schemas = JSON.parse(readFileSync("../api/openapi.json", "utf8")).components.schemas;

  it("mark exactly the keys the checked-in schema requires and cover every key", () => {
    for (const provider of NOTIFICATION_PROVIDERS) {
      const schema = schemas[SCHEMA[provider]!];
      const keys = CONFIG_KEYS[provider] as Record<string, string>;
      const required = Object.entries(keys).filter(([, spec]) => !spec.endsWith("?")).map(([key]) => key);
      expect([provider, required.sort()]).toEqual([provider, [...(schema.required ?? [])].sort()]);
      expect([provider, Object.keys(keys).sort()]).toEqual([provider, Object.keys(schema.properties).filter((k) => k !== "$schema").sort()]);
    }
  });

  it("start a new target with every required key and edit only known keys", () => {
    for (const provider of NOTIFICATION_PROVIDERS) {
      const keys = CONFIG_KEYS[provider] as Record<string, string>;
      const required = Object.entries(keys).filter(([, spec]) => !spec.endsWith("?")).map(([key]) => key);
      expect([provider, Object.keys(DEFAULTS[provider]).filter((k) => required.includes(k)).sort()]).toEqual([provider, required.sort()]);
      for (const field of FIELDS[provider]) expect([provider, field.key in keys]).toEqual([provider, true]);
    }
  });

  it("declare every wire event of the schema", () => {
    expect(Object.values(EVENT_KEYS).flat().sort()).toEqual(Object.keys(schemas.NotificationEventMap.properties).filter((k) => k !== "$schema").sort());
  });

  it("round-trip lists and headers through the text fields", () => {
    expect(fromText("list", "a@example.com, b@example.com\n\n", "email", "toAddresses")).toEqual(["a@example.com", "b@example.com"]);
    expect(toText("headers", { "X-One": "1", "X-Two": "a:b" })).toBe("X-One: 1\nX-Two: a:b");
    expect(fromText("headers", "X-One: 1\nX-Two: a:b", "generic", "customHeaders")).toEqual({ "X-One": "1", "X-Two": "a:b" });
    expect(fromText("number", "", "email", "smtpPort")).toBe(0);
    expect(fromText("host", " mail.example.com ", "email", "smtpHost")).toBe("mail.example.com");
  });
});
