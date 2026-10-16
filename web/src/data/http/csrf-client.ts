// Source: docklet-next/docs/design/identity.md
// Revision: af5fa9df01e730f7691d650985abd33d74f51ae5
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original CSRF bootstrap and HTTP adapter header handling.

import { AdapterOutcomeError } from "../capability";

const CSRF_HEADER = "X-CSRF-Token";

export interface CsrfClient {
  fetch: typeof fetch;
  token: () => Promise<string>;
}

export function createCsrfClient(baseUrl: string, fetcher: typeof fetch): CsrfClient {
  let token: string | undefined;
  let bootstrap: Promise<string> | undefined;

  function remember(response: Response): void {
    const next = response.headers.get(CSRF_HEADER);
    if (next) token = next;
  }

  async function requireToken(): Promise<string> {
    if (token) return token;
    bootstrap ??= (async () => {
      const response = await fetcher(`${baseUrl}/auth/mode`, { credentials: "same-origin", headers: { Accept: "application/json" } });
      if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: "http.error", status: response.status });
      remember(response);
      if (!token) throw new AdapterOutcomeError({ kind: "error", code: "http.csrf-token-missing", status: response.status });
      return token;
    })();
    try { return await bootstrap; } finally { bootstrap = undefined; }
  }

  return {
    token: requireToken,
    async fetch(input, init) {
      const request = input instanceof Request ? input : undefined;
      const method = (init?.method ?? request?.method ?? "GET").toUpperCase();
      const headers = new Headers(request?.headers);
      new Headers(init?.headers).forEach((value, key) => headers.set(key, value));
      if (!["GET", "HEAD", "OPTIONS"].includes(method)) headers.set(CSRF_HEADER, await requireToken());
      const response = await fetcher(input, { ...init, credentials: "same-origin", headers });
      remember(response);
      return response;
    },
  };
}
