// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

import { createCsrfClient } from "@/data/http/csrf-client";
import { decodeCertificateObservation } from "@/data/http/proxy-decoder";
import { ProxyError } from "@/data/http/proxy-api";
import type { components } from "@/api/generated/schema";

type Request = components["schemas"]["ProxyCertificateCheckRequest"];
/** The owning query's abort signal prevents a late read from reviving another identity. */
export async function checkCertificate(instance: string, input: Request, signal: AbortSignal) {
  const client = createCsrfClient("/api", (...args) => fetch(...args));
  signal.throwIfAborted();
  const response = await client.fetch(`/api/modules/proxy/instances/${encodeURIComponent(instance)}/certificate-check`, {
    method: "POST", signal, headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify(input),
  });
  const body: unknown = await response.json();
  signal.throwIfAborted();
  if (!response.ok) {
    const code = (body as { code?: unknown })?.code;
    throw new ProxyError(typeof code === "string" ? code : "http.error", undefined, response.status);
  }
  return decodeCertificateObservation(body);
}
