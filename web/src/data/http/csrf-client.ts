// Source: docklet-next/docs/design/identity.md
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original CSRF bootstrap and HTTP adapter header handling.

import { AdapterOutcomeError } from "../capability";

const CSRF_HEADER = "X-CSRF-Token";
const SAFE_METHODS = ["GET", "HEAD", "OPTIONS"];

export interface CsrfClient {
  fetch: typeof fetch;
  token: () => Promise<string>;
}

async function isCsrfFailure(response: Response): Promise<boolean> {
  if (response.status !== 403) return false;
  try {
    const body = await response.clone().json() as { code?: string; message?: string; detail?: string };
    return [body.code, body.message, body.detail].some(value => typeof value === "string" && /\bcsrf\b/i.test(value));
  } catch { return false; }
}

export function createCsrfClient(baseUrl: string, fetcher: typeof fetch): CsrfClient {
  let token: string | undefined;
  let bootstrap: Promise<void> | undefined;
  const basePath = new URL(baseUrl, "https://hub.example.com").pathname;

  async function send(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    const response = await fetcher(input, init);
    const next = response.headers.get(CSRF_HEADER);
    if (next) token = next;
    return response;
  }

  async function safeFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
    if (bootstrap) await bootstrap;
    if (token) return send(input, init);
    // Only one request may mint a cookie before the browser has a token pair.
    const first = send(input, init);
    const pending = first.then(() => undefined, () => undefined);
    bootstrap = pending;
    try { return await first; } finally { if (bootstrap === pending) bootstrap = undefined; }
  }

  async function requireToken(): Promise<string> {
    if (bootstrap) await bootstrap;
    if (token) return token;
    const response = await safeFetch(`${baseUrl}/auth/mode`, { credentials: "same-origin", headers: { Accept: "application/json" } });
    if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: response.status });
    if (!token) throw new AdapterOutcomeError({ kind: "error", code: "http.csrf-token-missing", status: response.status });
    return token;
  }

  return {
    token: requireToken,
    async fetch(input, init) {
      const request = input instanceof Request ? input : undefined;
      const method = (init?.method ?? request?.method ?? "GET").toUpperCase();
      const headers = new Headers(request?.headers);
      new Headers(init?.headers).forEach((value, key) => headers.set(key, value));
      const options = { ...init, credentials: "same-origin" as const, headers };
      const write = !SAFE_METHODS.includes(method);
      const attempt = () => send(request ? request.clone() : input, options);
      let response: Response;
      if (write) {
        headers.set(CSRF_HEADER, await requireToken());
        response = await attempt();
        if (await isCsrfFailure(response)) {
          token = undefined;
          headers.set(CSRF_HEADER, await requireToken());
          response = await attempt();
        }
      } else response = await safeFetch(input, options);
      const path = new URL(request?.url ?? String(input), "https://hub.example.com").pathname;
      const credentialCheck = ["/auth/password", "/auth/login"].some(suffix => path === basePath + suffix);
      if (response.status === 401 && !credentialCheck && typeof window !== "undefined") window.dispatchEvent(new Event("docklet:unauthorized"));
      return response;
    },
  };
}
