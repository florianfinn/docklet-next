// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 87bcad9941f6f9663bdf816d55e647faa57b922c
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { coversDomain, MIDDLEWARES, proxyStore, routeName, validDomain, validRoute, validRouteName, wildcardDomain, type ProxyCertificate, type ProxyRoute, type Resolver } from "./model";

interface Context { resolvers: Resolver[]; certificates: ProxyCertificate[] }
const middlewareTokens: Record<string, string[]> = { compress: ["encode", "gzip"], "strip-api": ["uri", "strip_prefix", "/api"], "custom-headers": ["header", "X-Proxy", "demo"] };
const middlewareLines = Object.fromEntries(Object.entries(middlewareTokens).map(([name, tokens]) => [name, tokens.join(" ")]));
const safeToken = (value: string) => /^[^\s{}#"`]+$/.test(value);
function upstream(value: string): boolean {
  try { const url = new URL(value); return ["http:", "https:"].includes(url.protocol) && !url.username && !url.password && url.pathname === "/" && !url.search && !url.hash && safeToken(value); } catch { return false; }
}
/** One site, an explicit path matcher (no implicit stripping), and ordered handlers. */
export function routeCaddyfile(route: ProxyRoute, context: Context = proxyStore.get()): string {
  if (route.middlewares.some((m) => !MIDDLEWARES.includes(m))) throw new Error("middlewareUnsupported");
  const lines = ["# docklet-route: " + routeName(route)];
  if (route.resolver && route.tls === "auto") lines.push("# docklet-issuer: " + route.resolver);
  if (route.certificateDomain !== undefined) lines.push("# docklet-certificate-domain: " + route.certificateDomain);
  const wildcard = route.tls === "auto" ? wildcardDomain(route) : undefined;
  const addresses = [...new Set([route.domain, ...(wildcard ? [wildcard, "*." + wildcard] : [])])];
  lines.push((route.tls === "off" ? "http://" : "") + addresses.join(", ") + " {");
  if (route.tls === "auto") {
    const resolver = context.resolvers.find((r) => r.proxy === route.proxy && r.name === route.resolver);
    if (route.resolver && !resolver && !route.caddyTls?.provider) throw new Error("issuerInvalid");
    const provider = route.caddyTls?.provider ?? (resolver?.challenge === "dns" ? resolver.provider : undefined);
    const credential = route.caddyTls?.credential ?? resolver?.credential;
    if (provider) {
      if (!/^[a-z0-9_]+$/.test(provider) || !/^[A-Z_][A-Z0-9_]*$/i.test(credential ?? "")) throw new Error("issuerInvalid");
      lines.push("  tls {", "    dns " + provider + " {env." + credential + "}", "  }");
    }
  } else if (route.tls === "custom") {
    const cert = context.certificates.find((c) => c.proxy === route.proxy && c.origin === "imported" && coversDomain(c.domains, route.domain));
    const certFile = route.caddyTls?.certFile ?? cert?.certFile ?? "", keyFile = route.caddyTls?.keyFile ?? cert?.keyFile ?? "";
    if (!safeToken(certFile) || !safeToken(keyFile)) throw new Error("importRequired");
    lines.push("  tls " + certFile + " " + keyFile);
  }
  const matched = !!route.path || addresses.length > 1;
  const indent = matched ? "    " : "  ";
  if (matched) {
    lines.push("  @docklet {");
    if (addresses.length > 1) lines.push("    host " + route.domain);
    if (route.path) lines.push("    path " + route.path + "*");
    lines.push("  }", "  handle @docklet {");
  }
  lines.push(indent + "route {");
  for (const middleware of route.middlewares) lines.push(indent + "  " + middlewareLines[middleware]);
  const targets = [route.url, ...route.backends].join(" ");
  if (route.backends.length) lines.push(indent + "  reverse_proxy " + targets + " {", indent + "    lb_policy round_robin", indent + "  }");
  else lines.push(indent + "  reverse_proxy " + targets);
  lines.push(indent + "}");
  if (matched) lines.push("  }");
  lines.push("}");
  return lines.join("\n") + "\n";
}

export function caddyProblem(route: ProxyRoute): "caddyValidation" | undefined {
  if (!validRoute(route) || !/^\/[a-z0-9/_.~%-]*$/i.test(route.path || "/") || ![route.url, ...route.backends].every(upstream) || route.middlewares.some((m) => !MIDDLEWARES.includes(m))) return "caddyValidation";

}

/** Consume only the form's grammar, then require an exact normalized round trip. */
export function routeFromCaddyfile(text: string, original: ProxyRoute): ProxyRoute {
  const lines = text.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
  let index = 0;
  const take = () => lines[index++] ?? "";
  const requireLine = (expected: string) => { if (take() !== expected) throw new Error("invalid"); };
  const name = take().match(/^# docklet-route: (.+)$/)?.[1] ?? "";
  if (!validRouteName(name)) throw new Error("invalid");
  const resolver = lines[index]?.startsWith("# docklet-issuer: ") ? take().slice(18) : "";
  if (resolver && !validRouteName(resolver)) throw new Error("invalid");
  const certificateDomain = lines[index]?.startsWith("# docklet-certificate-domain: ") ? take().slice("# docklet-certificate-domain: ".length) : undefined;
  if (certificateDomain !== undefined && !validDomain(certificateDomain, false)) throw new Error("invalid");
  const site = take().match(/^(http:\/\/)?(.+) \{$/);
  const addresses = site?.[2]?.split(", ") ?? [];
  const domain = addresses[0] ?? "";
  if (!site || !addresses.length || !addresses.every((d) => validDomain(d))) throw new Error("invalid");
  let tls: ProxyRoute["tls"] = site[1] ? "off" : "auto";
  let caddyTls: ProxyRoute["caddyTls"];
  if (lines[index] === "tls {") {
    take();
    const dns = take().match(/^dns ([a-z0-9_]+) \{env\.([A-Z_][A-Z0-9_]*)\}$/i);
    if (!dns || tls === "off") throw new Error("invalid");
    caddyTls = { provider: dns[1], credential: dns[2] };
    requireLine("}");
  } else if (lines[index]?.startsWith("tls ")) {
    const files = take().match(/^tls ([^\s{}#"`]+) ([^\s{}#"`]+)$/);
    if (!files || tls === "off") throw new Error("invalid");
    tls = "custom"; caddyTls = { certFile: files[1], keyFile: files[2] };
  }
  const matched = lines[index] === "@docklet {";
  let path = "";
  if (matched) {
    take();
    if (addresses.length > 1) requireLine("host " + domain);
    if (lines[index]?.startsWith("path ")) {
      const parsed = take().match(/^path (\/[^*\s]*)\*$/)?.[1];
      if (parsed === undefined) throw new Error("invalid");
      path = parsed;
    }
    requireLine("}"); requireLine("handle @docklet {");
  }
  requireLine("route {");
  const middlewares: string[] = [];
  while (Object.values(middlewareLines).includes(lines[index] ?? "")) {
    const line = take();
    const middleware = Object.keys(middlewareLines).find((key) => middlewareLines[key] === line)!;
    if (middlewares.includes(middleware)) throw new Error("invalid");
    middlewares.push(middleware);
  }
  const reverse = take().match(/^reverse_proxy (.+?)( \{)?$/);
  if (!reverse) throw new Error("invalid");
  const [url = "", ...backends] = reverse[1]!.split(" ");
  if (reverse[2]) { requireLine("lb_policy round_robin"); requireLine("}"); }
  if (!!reverse[2] !== !!backends.length) throw new Error("invalid");
  requireLine("}"); if (matched) requireLine("}"); requireLine("}");
  if (index !== lines.length) throw new Error("invalid");
  const route: ProxyRoute = { ...original, name, domain, path, url, backends, tls, resolver, certificateDomain, caddyTls, source: "file" };
  if (url !== original.url) { route.containerId = undefined; route.stackId = undefined; }
  const wildcard = tls === "auto" ? wildcardDomain(route) : undefined;
  if (wildcard && (!caddyTls?.provider || !coversDomain([wildcard, "*." + wildcard], domain))) throw new Error("invalid");
  if (!validRoute(route) || ![url, ...backends].every(upstream) || !/^\/[a-z0-9/_.~%-]*$/i.test(route.path || "/")) throw new Error("invalid");
  const normalized = routeCaddyfile(route).split("\n").map((line) => line.trim()).filter(Boolean);
  if (normalized.join("\n") !== lines.join("\n")) throw new Error("invalid");
  return route;
}
