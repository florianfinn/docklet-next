// @vitest-environment node
// Source: docklet-next/docs/design/identity.md
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original adapter integration test against the real Go CSRF middleware.

import { execFile } from "node:child_process";
import { mkdir, mkdtemp, rm } from "node:fs/promises";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import { promisify } from "node:util";
import { afterAll, beforeAll, expect, it } from "vitest";

import { createHttpAdapter } from "./http-adapter";
import { createCsrfClient } from "./csrf-client";

const baseUrl = "https://hub.example.com/api";
const run = promisify(execFile);
let bridgeDirectory: string | undefined;
let bridgeBinary: string;

beforeAll(async () => {
  // Share the binary with Go toolchains that run in a workspace-mounted container.
  const scratchDirectory = fileURLToPath(new URL("../../../../.scratch/", import.meta.url));
  await mkdir(scratchDirectory, { recursive: true });
  bridgeDirectory = await mkdtemp(join(scratchDirectory, "csrf-bridge-"));
  bridgeBinary = join(bridgeDirectory, process.platform === "win32" ? "csrf-bridge.exe" : "csrf-bridge");
  await run("go", ["build", "-o", bridgeBinary, "../backend/internal/middleware/testdata/csrf-bridge.go"], {
    env: { ...process.env, GOFLAGS: "-p=4" }, timeout: 180_000,
  });
}, 180_000);

afterAll(async () => {
  if (bridgeDirectory) await rm(bridgeDirectory, { recursive: true, force: true });
});

async function middlewareFetch(input: string | URL | Request, init?: RequestInit): Promise<Response> {
  const headers: Record<string, string[]> = {};
  new Headers(init?.headers).forEach((value, name) => { headers[name] = [value]; });
  const body = init?.body instanceof Blob ? await init.body.text() : String(init?.body ?? "");
  const request = { method: init?.method ?? "GET", path: new URL(String(input)).pathname, headers, body };
  const { stdout } = await run(bridgeBinary, [JSON.stringify(request)], { timeout: 20_000 });
  const response = JSON.parse(stdout) as { status: number; headers: Record<string, string[]>; body: string };
  const responseHeaders = new Headers();
  for (const [name, values] of Object.entries(response.headers)) for (const value of values) responseHeaders.append(name, value);
  return new Response(response.body, { status: response.status, headers: responseHeaders });
}

function browserTransport() {
  let cookie = "";
  const requests: { method: string; csrf: string | null }[] = [];
  const fetcher: typeof fetch = async (input, init) => {
    const headers = new Headers(init?.headers);
    if (cookie) headers.set("Cookie", cookie);
    const method = init?.method ?? "GET";
    if (!["GET", "HEAD", "OPTIONS"].includes(method)) headers.set("Origin", "https://hub.example.com");
    requests.push({ method, csrf: headers.get("X-CSRF-Token") });
    const response = await middlewareFetch(input, { ...init, headers });
    const issued = response.headers.getSetCookie()[0];
    if (issued) cookie = issued.split(";")[0]!;
    return response;
  };
  return { fetcher, requests };
}

it("sends the middleware-issued header for create, policy, restore, delete and XHR upload", async () => {
  const browser = browserTransport();
  let uploaded: Upload | undefined;
  class Upload {
    upload = { onprogress: null };
    withCredentials = false;
    responseText = "";
    status = 0;
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    headers = new Headers();
    url = "";
    open(_method: string, url: string) { this.url = url; }
    setRequestHeader(key: string, value: string) { this.headers.set(key, value); }
    send(file: File) {
      void browser.fetcher(this.url, { method: "POST", headers: this.headers, body: file }).then(async (response) => {
        this.status = response.status;
        this.responseText = await response.text();
        this.onload?.();
      }).catch(() => this.onerror?.());
    }
  }
  const adapter = createHttpAdapter({ baseUrl, fetch: browser.fetcher, uploadFactory: () => {
    uploaded = new Upload();
    return uploaded as unknown as XMLHttpRequest;
  } });
  expect((await adapter.createHubBackup()).id).toBe("example");
  expect(await adapter.saveHubBackupPolicies([])).toEqual([]);
  expect((await adapter.restoreHubBackup("example")).backupId).toBe("example");
  expect((await adapter.deleteHubBackup("example")).backupId).toBe("example");
  expect((await adapter.uploadHubBackup(new File(["example"], "archive.tar"), () => undefined)).backupId).toBe("example");
  expect(uploaded?.withCredentials).toBe(true);
  const writes = browser.requests.filter((entry) => entry.method !== "GET");
  expect(writes).toHaveLength(5);
  expect(writes.every((entry) => entry.csrf?.length === 43)).toBe(true);
  expect((await browser.fetcher(`${baseUrl}/backups`, { method: "POST" })).status).toBe(403);
});

it("shares concurrent token bootstrap", async () => {
  const browser = browserTransport();
  const client = createCsrfClient(baseUrl, browser.fetcher);
  const [first, second] = await Promise.all([client.token(), client.token()]);
  expect(first).toHaveLength(43);
  expect(second).toBe(first);
  expect(browser.requests).toHaveLength(1);
});

it.each([200, 403])("rejects writes when bootstrap HTTP %i provides no CSRF header", async (status) => {
  let calls = 0;
  const adapter = createHttpAdapter({ baseUrl, fetch: async () => {
    ++calls;
    return new Response("{}", { status });
  } });
  await expect(adapter.createHubBackup()).rejects.toMatchObject({ outcome: { kind: "error", status } });
  expect(calls).toBe(1);
});
