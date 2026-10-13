import type { Host } from "@/domain/model";

/**
 * The host's own address without scheme, path and agent port, as containers in
 * host network mode share it. Only direct hosts have a known address; edge
 * agents dial out, so their host address is unknown to the hub.
 */
export function hostAddress(host: Host | undefined): string | null {
  if (!host || host.connection !== "direct") return null;
  const raw = host.address
    .trim()
    .replace(/^[a-z][a-z0-9+.-]*:\/\//i, "")
    .replace(/[/?#].*$/, "");
  const bracketed = /^\[([^\]]+)\](?::\d+)?$/.exec(raw);
  if (bracketed) return bracketed[1] ?? null;
  // A bare IPv6 address has several colons and no port to strip.
  if ((raw.match(/:/g) ?? []).length > 1) return raw || null;
  return raw.replace(/:\d+$/, "") || null;
}
