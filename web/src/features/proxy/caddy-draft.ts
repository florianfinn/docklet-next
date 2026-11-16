// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import type { Route } from "./types";

/** Build the small Easy shape; the server alone decides whether this site is valid. */
export function newCaddySite(name: string, domain: string, upstream: string, tls: boolean): string {
  return `# docklet-route: ${name}\n${tls ? "" : "http://"}${domain} {\n  route {\n    reverse_proxy ${upstream}\n  }\n}\n`;
}
/** Change only displayed leaves; keep unrepresented directives, TLS and additional upstreams. */
export function patchCaddySite(text: string, route: Route, name: string, domain: string, upstream: string): string {
  let next = text;
  // Indentation is accepted by the engine's site grammar, so both patterns keep it.
  if (name !== route.name)
    next = next.replace(/^([ \t]*)# docklet-route: .*$/m, (_line, indent: string) => `${indent}# docklet-route: ${name}`);
  if (domain !== route.rule) {
    // The site header is the first line that is neither blank nor a comment.
    const header = /^(?![ \t]*(?:#|$))[^\n]*\{[ \t]*$/m;
    next = next.replace(header, line => line.replace(route.rule, domain));
    next = next.replace(
      /^(\s*host )([^\n]+)$/m,
      (_line, prefix: string, hosts: string) =>
        prefix +
        hosts
          .split(" ")
          .map(h => (h === route.rule ? domain : h))
          .join(" "),
    );
  }
  const old = route.upstreams?.[0];
  if (old && old !== upstream)
    next = next.replace(
      /^(\s*reverse_proxy )([^\n]+)$/m,
      (_line, prefix: string, targets: string) =>
        prefix +
        targets
          .split(" ")
          .map(u => (u === old ? upstream : u))
          .join(" "),
    );
  return next;
}
