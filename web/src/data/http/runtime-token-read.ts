// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original consumer classification of token-bootstrap transport failures.

const tokenErrors = new WeakSet<object>();

/** Classification retains the caller's original constructor, identity and message. */
export function isRuntimeTokenReadError(error: unknown): boolean {
  return error !== null && (typeof error === "object" || typeof error === "function") && tokenErrors.has(error);
}

/** Preserve CSRF policy while identifying raw bootstrap failures at their source. */
export function runtimeTokenReads(baseUrl: string, fetcher: typeof fetch): typeof fetch {
  const tokenPath = new URL(`${baseUrl}/auth/mode`, "https://hub.example.com").pathname;
  return async (input, init) => {
    try { return await fetcher(input, init); }
    catch (error) {
      const path = new URL(input instanceof Request ? input.url : String(input), "https://hub.example.com").pathname;
      if (error !== null && (typeof error === "object" || typeof error === "function")) {
        if (path === tokenPath) tokenErrors.add(error);
        else tokenErrors.delete(error);
      }
      throw error;
    }
  };
}
