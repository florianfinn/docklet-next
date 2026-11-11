// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original extraction of the HTTP adapter's request chain (dispatch gate, host admission, identity reads, CSRF and the non-owned 401 wrapper).

import { createCsrfClient } from "./csrf-client";
import { createDispatchGate } from "./dispatch-gate";
import { createHostReadAdmission } from "./host-read-admission";
import { createHostReadPolicy } from "./host-read-policy";
import { createIdentityReads } from "./identity-reads";
import { createMaintenanceObserver } from "./maintenance-observer";
import { runtimeTokenReads } from "./runtime-token-read";

type ChainOptions = {
  baseUrl: string;
  fetch: typeof fetch;
  /** Wait while a logout runs; refuse once it signed out. */
  admit: () => Promise<void>;
  /** Whether a logout runs or the session signed out. */
  closing: () => boolean;
  generation: () => number;
  /** A non-owned 401 of the current identity ends it. */
  suspend: () => void;
};

/** client.fetch → CSRF → runtime token reads → identity reads → host admission → dispatch gate → network. */
export function createTransportChain({ baseUrl, fetch: rawFetch, admit, closing, generation, suspend }: ChainOptions) {
  const maintenance = createMaintenanceObserver(`${baseUrl}/backups/maintenance`, rawFetch);
  const hostPolicy = createHostReadPolicy();
  const network = createHostReadAdmission(createDispatchGate(rawFetch, baseUrl, maintenance, hostPolicy), baseUrl);
  // Admitted reads still waiting for their answer; a logout lets them land before the session ends.
  const answering = new Set<Promise<void>>();
  // Below CSRF queuing and the transport retry, so each physical attempt is admitted on its own.
  const identityReads = createIdentityReads(async (input, init) => {
    const path = new URL(input instanceof Request ? input.url : String(input), (typeof window === "undefined" ? "https://hub.example.com" : window.location.href)).pathname;
    const method = (init?.method ?? (input instanceof Request ? input.method : "GET")).toUpperCase();
    const read = (method === "GET" || method === "HEAD") && !path.includes("/auth/");
    if (read && closing()) await admit();
    const answer = network(input, init);
    if (read) {
      const landed = answer.then(() => undefined, () => undefined);
      answering.add(landed);
      void landed.then(() => answering.delete(landed));
    }
    return answer;
  });
  const rawClient = createCsrfClient(baseUrl, runtimeTokenReads(baseUrl, identityReads.fetch));
  const client = { token: rawClient.token, fetch: (async (input, init) => {
    const version = generation();
    const path = new URL(input instanceof Request ? input.url : String(input), (typeof window === "undefined" ? "https://hub.example.com" : window.location.href)).pathname;
    const response = await rawClient.fetch(input, init);
    // Auth and activity/runtime consumers already own their authoritative 401 path.
    const ownedRejection = ["/auth/login", "/auth/password", "/auth/me"].some((suffix) => path.endsWith(suffix)) || /\/(activities|runtime-runs)(\/|$)|\/(capabilities|environments)$/.test(path);
    if (version === generation() && response.status === 401 && !ownedRejection) {
      const body = await response.arrayBuffer();
      if (version === generation()) suspend();
      return new Response(body, { status: response.status, headers: response.headers });
    }
    return response;
  }) as typeof fetch };
  return { maintenance, hostPolicy, identityReads, answering, client };
}
