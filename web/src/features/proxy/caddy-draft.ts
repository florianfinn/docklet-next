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
  // Indentation and CRLF line ends are accepted by the engine's site grammar; every
  // pattern keeps both and never lets a token include the carriage return.
  if (name !== route.name)
    next = next.replace(
      /^([ \t]*)# docklet-route: [^\r\n]*(\r?)$/m,
      (_line, indent: string, cr: string) => `${indent}# docklet-route: ${name}${cr}`,
    );
  if (domain !== route.rule) {
    // The site header is the first line that is neither blank nor a comment.
    const header = /^(?![ \t]*(?:#|\r?$))[^\r\n]*\{[ \t]*\r?$/m;
    next = next.replace(header, line => line.replace(route.rule, domain));
    next = next.replace(
      /^([ \t]*host )([^\r\n]+?)(\r?)$/m,
      (_line, prefix: string, hosts: string, cr: string) =>
        prefix +
        hosts
          .split(" ")
          .map(h => (h === route.rule ? domain : h))
          .join(" ") +
        cr,
    );
  }
  const old = route.upstreams?.[0];
  if (old && old !== upstream)
    next = next.replace(
      /^([ \t]*reverse_proxy )([^\r\n]+?)(\r?)$/m,
      (_line, prefix: string, targets: string, cr: string) => {
        // Easy edits the first upstream only: replace that position, never equal later tokens.
        const parts = targets.split(" "),
          first = parts.indexOf(old);
        if (first >= 0) parts[first] = upstream;
        return prefix + parts.join(" ") + cr;
      },
    );
  return next;
}
