// Source: docklet-next/docs/design/runtime-control.md
// Revision: 225dd4d1d1dc7fb16adc91179d3d9d6d675d2ced
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original log mapping, byte bounds and arrival-order regressions.

import { describe, expect, it } from "vitest";
import { boundLogLine, LogBuffer, logBytes, LOG_LINE_BYTES, LOG_MAX_BYTES, LOG_MAX_LINES } from "../log-buffer";
import { logAddress, mapLog } from "./log-mapping";
import { wireLog } from "./logs.test-support";
const now = () => "2026-02-03T04:05:06Z";
const map = (value: unknown) => mapLog(value, "example-scope", 1, "example-container", now);

describe("runtime log mapping", () => {
  it.each(["0", "direct", "edge"])("addresses the existing hub route for %s", (host) => {
    for (const kind of ["stack", "container"] as const) {
      const address = logAddress("https://hub.example.com/api", kind, `${host}~name /?#`);
      const url = new URL(address.url);
      expect(url.protocol).toBe("wss:");
      expect(url.pathname).toBe(`/api/environments/${host}/ws/${kind === "stack" ? "projects" : "containers"}/name%20%2F%3F%23/logs`);
      expect(Object.fromEntries(url.searchParams)).toEqual({ format: "json", follow: "true", tail: "100", timestamps: "true", batched: "false" });
      expect(url.username).toBe(""); expect(url.password).toBe("");
    }
    expect(logAddress("/api", "stack", "host /?~raw").url).toContain("host%20%2F%3F");
  });
  it("accepts old optional omissions and uses known source and receive-time fallback", () => {
    expect(map({ seq: 1, message: "  raw text  " })).toMatchObject({ service: "example-container", ts: now(), level: "info", message: "  raw text  ", stream: "unknown", truncated: false });
    expect(map({ ...wireLog(2), service: "worker", stream: "stderr", level: "stderr" })).toMatchObject({ service: "worker", stream: "stderr", level: "info" });
    expect(map({ ...wireLog(2), level: "error" }).level).toBe("error");
    expect(map({ ...wireLog(2), message: "ERROR fatal warning" }).level).toBe("info");
  });
  it.each([null, {}, { seq: 0, message: "x" }, { seq: 1.1, message: "x" }, { seq: 1, message: 0 }, { seq: 1, message: "x", timestamp: "wrong" }, { seq: 1, message: "x", stream: "invented" }, { seq: 1, message: "x", truncated: "true" }, { seq: 1, message: "x", service: 4 }])("refuses malformed mandatory or present optional fields %#", (value) => {
    expect(() => map(value)).toThrow();
  });
  it.each(["1", "2026-01-02", "2026-02-30T03:04:05Z", "2026-01-02T24:00:00Z"])("rejects non-RFC3339 or impossible timestamps %s", (timestamp) => {
    expect(() => map({ ...wireLog(1), timestamp })).toThrow();
  });
  it("uses attempt and full resource scope instead of timestamp or text as identity", () => {
    const one = mapLog(wireLog(1, "same"), "host/stack/raw", 1, "", now);
    const two = mapLog(wireLog(1, "same"), "host/stack/raw", 2, "", now);
    expect(one.id).not.toBe(two.id);
    expect(two.id).not.toBe(mapLog(wireLog(1, "same"), "host/container/raw", 2, "", now).id);
  });
  it.each(["界", "😀", "\u0000", '"\\'])("bounds serialized single-line UTF-8 including escaping %s", (character) => {
    const result = boundLogLine({ ...map(wireLog(1)), message: character.repeat(100000) });
    expect(logBytes(result)).toBeLessThanOrEqual(LOG_LINE_BYTES); expect(result.truncated).toBe(true);
    expect(result.message.endsWith("\ud83d")).toBe(false);
  });
  it("bounds both a published snapshot and the next frame during unlimited input", () => {
    const buffer = new LogBuffer();
    for (let i = 1; i <= 3000; ++i) buffer.append(map(wireLog(i)));
    const published = buffer.snapshot({ status: "live", reconnect: false });
    expect(published.length).toBeGreaterThan(0); expect(buffer.dropped).toBeGreaterThan(0);
    for (let i = 3001; i <= 6000; ++i) buffer.append(map(wireLog(i, "界".repeat(15000))));
    const next = buffer.snapshot({ status: "live", reconnect: false });
    expect(published.length + next.length).toBeLessThanOrEqual(LOG_MAX_LINES);
    expect(published.reduce((sum, row) => sum + logBytes(row), 0) + buffer.bytes).toBeLessThanOrEqual(LOG_MAX_BYTES);
    expect(next.at(-1)?.seq).toBe(6000);
  });
});
