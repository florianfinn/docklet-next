// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { PROXY_ISSUERS } from "@/data/demo/proxy-issuers";
import { parse, stringify } from "yaml";
import { createStore, useStore } from "@/shared/lib/store";

export interface ProxyRoute {
  id: string; name?: string; proxy: string; domain: string; path: string; url: string; backends: string[];
  containerId?: string; stackId?: string; hostId: string; source: "file" | "labels";
  tls: "off" | "auto" | "custom"; resolver: string; middlewares: string[];
  enabled: boolean; redirect: boolean; certificateDomain?: string; labNames?: string[];
  caddyTls?: { provider?: string; credential?: string; certFile?: string; keyFile?: string };
}
export interface ProxyCertificate {
  id: string; proxy: string; name: string; domains: string[]; issuer: string; expires: string;
  origin: "automatic" | "imported"; resolver: string; certFile: string; keyFile: string;
  managed: boolean; status: "valid" | "pending" | "expiresSoon";
}
export interface Resolver {
  id: string; proxy: string; name: string; email: string;
  challenge: "dns" | "http"; provider: string; credential: string;
}
export type ProxyKind = "traefik" | "caddy";
export interface ProxyInstance { id: string; name: string; hostId: string; kind: ProxyKind }
export function proxyKindFromImage(image: string): ProxyKind | undefined {
  return /(?:^|\/)caddy(?=[:@]|$)/i.test(image) ? "caddy" : /(?:^|\/)traefik(?=[:@]|$)/i.test(image) ? "traefik" : undefined;
}
export const proxyKindLabel = (kind: ProxyKind) => kind === "caddy" ? "Caddy" : "Traefik";
export function migrateInstances(instances: (Omit<ProxyInstance, "kind"> & { kind?: ProxyKind })[]): ProxyInstance[] {
  return instances.map((instance) => ({ ...instance, kind: instance.kind ?? "traefik" }));
}
export const INSTANCES: ProxyInstance[] = [
  { id: "atlas", name: "Traefik · Atlas", hostId: "atlas", kind: "traefik" },
  { id: "boreal", name: "Traefik · Boreal", hostId: "boreal", kind: "traefik" },
];
INSTANCES.push({ id: "caddy-example", name: "Caddy · Example", hostId: "atlas", kind: "caddy" });
export const MIDDLEWARES = ["compress", "strip-api", "custom-headers"];
const base = { proxy: "atlas", path: "", backends: [], source: "file" as const, tls: "auto" as const, resolver: "dns-primary", middlewares: ["compress"], enabled: true, redirect: true };
export const INITIAL_ROUTES: ProxyRoute[] = [
  { ...base, id: "media", domain: "media.example.com", url: "http://jellyfin:8096", hostId: "atlas", containerId: "c-media-jellyfin", stackId: "media" },
  { ...base, id: "documents", domain: "docs.example.com", url: "http://webserver:8000", hostId: "atlas", containerId: "c-paperless-webserver", stackId: "paperless", source: "labels", middlewares: ["compress", "custom-headers"] },
  { ...base, id: "home", domain: "home.example.com", url: "http://homeassistant:8123", hostId: "atlas", containerId: "c-home-homeassistant", stackId: "home", middlewares: [] },
  { ...base, id: "archive", domain: "archive.example.net", url: "http://192.0.2.20:8080", hostId: "atlas", tls: "custom", middlewares: ["custom-headers"] },
  { ...base, id: "api", domain: "api.example.com", path: "/api", url: "http://192.0.2.30:8080", backends: ["http://192.0.2.31:8080"], hostId: "atlas", middlewares: ["strip-api", "compress"] },
  { ...base, id: "preview", domain: "preview.example.com", url: "http://192.0.2.40:8080", hostId: "atlas", enabled: false, tls: "off", redirect: false, middlewares: [] },
  { ...base, proxy: "boreal", id: "cloud", domain: "files.example.net", url: "http://nextcloud:80", hostId: "boreal", containerId: "c-cloud-nextcloud", stackId: "cloud", source: "labels", resolver: "http-primary" },
  { ...base, proxy: "boreal", id: "status", domain: "status.example.net", url: "http://uptime-kuma:3001", hostId: "boreal", containerId: "c-uptime-kuma", resolver: "http-primary" },
];
INITIAL_ROUTES.push(
  { ...base, id: "caddy-app", proxy: "caddy-example", domain: "app.example.com", url: "http://192.0.2.50:8080", hostId: "atlas", resolver: "" },
  { ...base, id: "caddy-archive", proxy: "caddy-example", domain: "archive.example.net", url: "http://192.0.2.51:8080", hostId: "atlas", resolver: "", tls: "custom" },
);
const certBase = { certFile: "", keyFile: "", managed: true, status: "valid" as const };
export const INITIAL_CERTIFICATES: ProxyCertificate[] = [
  { ...certBase, id: "wildcard", proxy: "atlas", name: "example.com", domains: ["example.com", "*.example.com"], issuer: PROXY_ISSUERS.acme, expires: "2027-01-15", origin: "automatic", resolver: "dns-primary" },
  { ...certBase, id: "archive-cert", proxy: "atlas", name: "Archive", domains: ["archive.example.net"], issuer: PROXY_ISSUERS.imported, expires: "2026-11-12", origin: "imported", resolver: "", certFile: "/certs/archive.crt", keyFile: "/certs/archive.key", status: "expiresSoon" },
  { ...certBase, id: "files-cert", proxy: "boreal", name: "Files", domains: ["files.example.net"], issuer: PROXY_ISSUERS.acme, expires: "2027-01-20", origin: "automatic", resolver: "http-primary" },
  { ...certBase, id: "status-cert", proxy: "boreal", name: "Status", domains: ["status.example.net"], issuer: PROXY_ISSUERS.acme, expires: "2027-01-20", origin: "automatic", resolver: "http-primary" },
];
INITIAL_CERTIFICATES.push(
  { ...certBase, id: "caddy-auto", proxy: "caddy-example", name: "app.example.com", domains: ["app.example.com"], issuer: PROXY_ISSUERS.acme, expires: "2027-01-20", origin: "automatic", resolver: "" },
  { ...certBase, id: "caddy-import", proxy: "caddy-example", name: "archive.example.net", domains: ["archive.example.net"], issuer: PROXY_ISSUERS.imported, expires: "2027-01-20", origin: "imported", resolver: "", certFile: "/certs/archive.crt", keyFile: "/certs/archive.key" },
);
export interface ProxyConnection {
  mode: "discovered" | "manual"; container: string; host: string; image: string;
  configPath: string; apiUrl: string; lab?: ProxyKind;
}
export const DISCOVERED_PROXIES = [
  { id: "atlas-edge", host: "atlas", container: "edge-traefik-1", image: "traefik:v3.4", configPath: "/etc/traefik/dynamic", apiUrl: "http://traefik:8080", mode: "discovered" as const },
  { id: "boreal-edge", host: "boreal", container: "gateway-traefik-1", image: "traefik:v3.4", configPath: "/etc/traefik/dynamic", apiUrl: "http://gateway:8080", mode: "discovered" as const },
];
DISCOVERED_PROXIES.push({ id: "atlas-caddy", host: "atlas", container: "caddy-1", image: "caddy:2.10.2", configPath: "/etc/caddy", apiUrl: "", mode: "discovered" });
/** Config names are independent of both Docklet route IDs and Docker container IDs. */
export function routeName(route: ProxyRoute): string {
  if (route.name !== undefined) return route.name.trim();
  let target = "";
  try { target = new URL(route.url).hostname; } catch { /* A new draft may have no target yet. */ }
  return ((route.domain || (route.stackId ? route.stackId + "-" : "") + target || "route") + (route.path ? "-" + route.path : ""))
    .toLowerCase().replaceAll("*", "wildcard").replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 64) || "route";
}
export function validRouteName(name: string): boolean {
  return /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$/.test(name);
}
export function availableRouteName(name: string, proxy: string, routes: ProxyRoute[]): string {
  const used = new Set(routes.filter((r) => r.proxy === proxy).map(routeName));
  let next = name;
  for (let suffix = 2; used.has(next); suffix++) next = name.slice(0, 70) + "-" + suffix;
  return next;
}
/** Fill names for previously saved browser fixtures without changing their internal identity. */
export function migrateRouteNames(routes: ProxyRoute[]): ProxyRoute[] {
  const named: ProxyRoute[] = [];
  for (const route of routes) named.push({ ...route, name: availableRouteName(routeName(route), route.proxy, named) });
  return named;
}
export function routeNameConflict(route: ProxyRoute, routes: ProxyRoute[]): boolean {
  return routes.some((r) => r.proxy === route.proxy && r.id !== route.id && routeName(r) === routeName(route));
}
export function duplicateRoute(route: ProxyRoute, routes: ProxyRoute[]): ProxyRoute {
  return { ...route, id: crypto.randomUUID(), name: availableRouteName(routeName(route), route.proxy, routes), domain: "", source: "file", labNames: undefined };
}
interface State { instances: ProxyInstance[]; connections: Record<string, ProxyConnection>; routes: ProxyRoute[]; certificates: ProxyCertificate[]; resolvers: Resolver[]; instance: string }
export const proxyStore = createStore<State>({
  instances: INSTANCES, connections: {}, routes: migrateRouteNames(INITIAL_ROUTES), certificates: INITIAL_CERTIFICATES, instance: "atlas",
  resolvers: [
    { id: "dns-primary", proxy: "atlas", name: "dns-primary", email: "admin@example.com", challenge: "dns", provider: "cloudflare", credential: "CF_DNS_API_TOKEN" },
    { id: "http-primary", proxy: "boreal", name: "http-primary", email: "admin@example.net", challenge: "http", provider: "", credential: "" },
  ],
}, { key: "docklet.proxy-concept", version: 1, merge: (stored, defaults) => { const state = { ...defaults, ...(stored as Partial<State>) }; return { ...state, instances: migrateInstances(state.instances), routes: migrateRouteNames(state.routes) }; } });
export const useProxy = () => useStore(proxyStore, (state) => state);
export function upsertRoute(route: ProxyRoute) {
  if (!validRouteName(routeName(route)) || routeNameConflict(route, proxyStore.get().routes)) throw new Error("route-name");
  route = { ...route, name: routeName(route) };
  proxyStore.set((s) => ({ ...s, routes: s.routes.some((r) => r.id === route.id) ? s.routes.map((r) => r.id === route.id ? route : r) : [...s.routes, route] }));
}
export function newRoute(proxy: string): ProxyRoute {
  return { ...base, id: crypto.randomUUID(), proxy, hostId: proxyStore.get().connections[proxy]?.host ?? proxy, domain: "", url: "", resolver: proxyStore.get().resolvers.find((r) => r.proxy === proxy)?.name ?? "" };
}
export function coversDomain(domains: string[], domain: string): boolean {
  return domains.some((d) => d === domain || (d.startsWith("*.") && domain.endsWith(d.slice(1)) && domain.split(".").length === d.split(".").length));
}
export function matchingCertificates(route: ProxyRoute, certs: ProxyCertificate[]): ProxyCertificate[] {
  return route.tls === "off" ? [] : certs.filter((c) => c.proxy === route.proxy && coversDomain(c.domains, route.domain));
}
export function validUrl(value: string): boolean {
  try { return ["http:", "https:"].includes(new URL(value).protocol); } catch { return false; }
}
export function validDomain(value: string, wildcard = true): boolean {
  const domain = wildcard && value.startsWith("*.") ? value.slice(2) : value;
  return domain.length <= 253 && domain.split(".").length > 1 && domain.split(".").every((part) => /^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$/i.test(part));
}
export function wildcardDomain(route: ProxyRoute): string | undefined {
  return route.certificateDomain ?? (route.domain.startsWith("*.") ? route.domain.slice(2) : undefined);
}
export function wildcardProblem(route: ProxyRoute, resolvers: Resolver[]): "dnsRequired" | "coverageMismatch" | undefined {
  const domain = wildcardDomain(route);
  if (route.tls !== "auto" || !domain) return;
  if (!(proxyStore.get().instances.find((p) => p.id === route.proxy)?.kind === "caddy" && route.caddyTls?.provider) && !resolvers.some((r) => r.proxy === route.proxy && r.name === route.resolver && r.challenge === "dns")) return "dnsRequired";
  if (!coversDomain([domain, "*." + domain], route.domain)) return "coverageMismatch";
}
export function routeRule(route: ProxyRoute): string {
  const quote = String.fromCharCode(96);
  const host = route.domain.startsWith("*.")
    ? "HostRegexp(" + quote + "^[a-z0-9-]+\\." + route.domain.slice(2).replaceAll(".", "\\.") + "$" + quote + ")"
    : "Host(" + quote + route.domain + quote + ")";
  return host + (route.path ? " && PathPrefix(" + quote + route.path + quote + ")" : "");
}
export function validRoute(route: ProxyRoute): boolean {
  return validRouteName(routeName(route)) && validDomain(route.domain) && (route.certificateDomain === undefined || validDomain(route.certificateDomain, false)) && (!route.path || route.path.startsWith("/")) && validUrl(route.url) && route.backends.every(validUrl);
}
export function routeYaml(route: ProxyRoute): string {
  const name = routeName(route);
  const wildcard = wildcardDomain(route);
  const router = {
    rule: routeRule(route),
    entryPoints: [route.tls === "off" ? "web" : "websecure"], service: name,
    ...(route.middlewares.length ? { middlewares: route.middlewares } : {}),
    ...(route.tls !== "off" ? { tls: route.tls === "auto" ? { certResolver: route.resolver, ...(wildcard ? { domains: [{ main: wildcard, sans: ["*." + wildcard] }] } : {}) } : {} } : {}),
  };
  return stringify({ http: { routers: { [name]: router }, services: { [name]: { loadBalancer: { servers: [route.url, ...route.backends].map((url) => ({ url })) } } } } });
}
/** The prototype text editor handles the same single-router shape as its form. */
export function routeFromYaml(text: string, original: ProxyRoute): ProxyRoute {
  const doc = parse(text);
  const routerNames = Object.keys(doc?.http?.routers ?? {});
  const serviceNames = Object.keys(doc?.http?.services ?? {});
  if (routerNames.length !== 1 || serviceNames.length !== 1) throw new Error("invalid");
  const key = routerNames[0]!;
  const router = doc.http.routers[key];
  if (router?.service !== key || serviceNames[0] !== key) throw new Error("invalid");
  // Previously generated YAML used the internal route ID at all three positions.
  const name = key === original.id && /^[0-9a-f]{8}-[0-9a-f-]{27}$/i.test(key) ? routeName(original) : key;
  if (!validRouteName(name)) throw new Error("invalid");
  const servers = doc?.http?.services?.[router?.service]?.loadBalancer?.servers;
  if (typeof router?.rule !== "string" || !Array.isArray(servers) || !servers.length || servers.some((s) => typeof s?.url !== "string") || (router.middlewares && (!Array.isArray(router.middlewares) || router.middlewares.some((m: unknown) => typeof m !== "string")))) throw new Error("invalid");
  let domain = router.rule.match(/Host\([\x60"]([^\x60"]+)[\x60"]\)/)?.[1] ?? "";
  if (!domain) {
    const expression = router.rule.match(/HostRegexp\([\x60"]([^\x60"]+)[\x60"]\)/)?.[1] ?? "";
    const prefix = "^[a-z0-9-]+\\.";
    if (!expression.startsWith(prefix) || !expression.endsWith("$")) throw new Error("invalid");
    domain = "*." + expression.slice(prefix.length, -1).replaceAll("\\.", ".");
  }
  const certificateDomain = router.tls?.domains?.[0]?.main;
  if (certificateDomain !== undefined && (typeof certificateDomain !== "string" || !validDomain(certificateDomain, false))) throw new Error("invalid");
  const path = router.rule.match(/PathPrefix\([\x60"]([^\x60"]+)[\x60"]\)/)?.[1] ?? "";
  const url = servers[0].url as string;
  const route: ProxyRoute = { ...original, name, domain, path, url, certificateDomain, backends: servers.slice(1).map((s: { url: string }) => s.url), tls: router.tls ? Object.hasOwn(router.tls, "certResolver") ? "auto" : "custom" : "off", resolver: router.tls?.certResolver ?? "", middlewares: router.middlewares ?? [] };
  if (url !== original.url) { route.containerId = undefined; route.stackId = undefined; }
  if (!validRoute(route)) throw new Error("invalid");
  return route;
}

export { routeCaddyfile, routeFromCaddyfile } from "./caddyfile";
