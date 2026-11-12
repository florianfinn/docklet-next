// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 87bcad9941f6f9663bdf816d55e647faa57b922c
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { describe, expect, it } from "vitest";
import { caddyProblem, routeCaddyfile, routeFromCaddyfile } from "./caddyfile";
import { INITIAL_ROUTES, migrateInstances, proxyKindFromImage, proxyStore, routeName, type ProxyRoute } from "./model";
const original: ProxyRoute = { ...INITIAL_ROUTES[0]!, proxy: "caddy-example", resolver: "", name: "example-app" };
const dns = { ...original, resolver: "dns-example", caddyTls: { provider: "cloudflare", credential: "DNS_API_TOKEN" } };

describe("proxy kind and legacy browser state", () => {
  it("migrates kind without changing IDs, host assignments or names", () => {
    const legacy = [{ id: "saved-instance", name: "My proxy", hostId: "atlas" }];
    expect(migrateInstances(legacy)).toEqual([{ ...legacy[0], kind: "traefik" }]);
    expect(migrateInstances(migrateInstances(legacy))).toEqual(migrateInstances(legacy));
    expect(migrateInstances([{ ...legacy[0]!, kind: "caddy" }])[0]!.kind).toBe("caddy");
  });
  it.each([["caddy:2.10.2", "caddy"], ["docker.io/library/caddy:2.10.2", "caddy"], ["traefik:v3.4.5", "traefik"], ["not-caddy:1", undefined], ["example.com/caddy-sidecar:1", undefined]])("identifies %s from its image", (image, kind) => {
    expect(proxyKindFromImage(image!)).toBe(kind);
  });
});
describe("bounded Caddyfile round trips", () => {
  it.each(["auto", "off"] as const)("keeps %s route names, identities and targets", (tls) => {
    const route = { ...original, tls, path: "/api", backends: ["http://192.0.2.80:8080"], middlewares: ["strip-api", "compress", "custom-headers"] };
    const text = routeCaddyfile(route);
    expect(text.startsWith("# docklet-route: example-app\n")).toBe(true);
    expect(text.includes(tls === "off" ? "http://media.example.com {" : "media.example.com {")).toBe(true);
    expect(text.includes("path /api*")).toBe(true);
    expect(text.includes("handle_path")).toBe(false);
    expect(text.includes("lb_policy round_robin")).toBe(true);
    const parsed = routeFromCaddyfile(text, route);
    expect(parsed).toMatchObject({ id: route.id, name: route.name, tls, path: "/api", url: route.url, backends: route.backends, middlewares: route.middlewares });
    expect(routeCaddyfile(parsed)).toBe(text);
  });
  it("uses the configured DNS provider and stores credential names only", () => {
    const route = { ...original, proxy: "atlas", resolver: "dns-primary", domain: "*.example.com" };
    const text = routeCaddyfile(route);
    expect(text.includes("dns cloudflare {env.CF_DNS_API_TOKEN}")).toBe(true);
    expect(text.includes("*.example.com, example.com {")).toBe(true);
    expect(text.includes("HostRegexp")).toBe(false);
    expect(routeCaddyfile(routeFromCaddyfile(text, route))).toBe(text);
  });
  it("keeps exact routing separate from wildcard certificate subjects in one site block", () => {
    const route = { ...dns, certificateDomain: "example.com" };
    const text = routeCaddyfile(route);
    expect(text.includes("media.example.com, example.com, *.example.com {")).toBe(true);
    expect(text.includes("host media.example.com")).toBe(true);
    expect(routeFromCaddyfile(text, original)).toMatchObject({ domain: original.domain, certificateDomain: "example.com", resolver: "dns-example", caddyTls: dns.caddyTls });
    expect(routeCaddyfile(routeFromCaddyfile(text, original))).toBe(text);
  });
  it("maps imported certificate files and retains Advanced file edits", () => {
    const route = { ...original, domain: "archive.example.net", tls: "custom" as const };
    const text = routeCaddyfile(route);
    expect(text.includes("tls /certs/archive.crt /certs/archive.key")).toBe(true);
    const parsed = routeFromCaddyfile(text.replaceAll("/certs/archive", "/certs/replaced"), route);
    expect(parsed.caddyTls).toEqual({ certFile: "/certs/replaced.crt", keyFile: "/certs/replaced.key" });
    expect(routeCaddyfile(parsed).includes("/certs/replaced.crt")).toBe(true);
    expect(() => routeCaddyfile({ ...route, domain: "missing.example.net" })).toThrow("importRequired");
  });
  it("accepts renames and target changes without reassigning internal identity", () => {
    const text = routeCaddyfile(original).replace("docklet-route: example-app", "docklet-route: renamed").replace(original.url, "http://192.0.2.70:8080");
    expect(routeFromCaddyfile(text, original)).toMatchObject({ id: original.id, name: "renamed", url: "http://192.0.2.70:8080", containerId: undefined, stackId: undefined });
    expect(routeCaddyfile({ ...original, containerId: "recreated" })).toBe(routeCaddyfile(original));
    expect(routeName(routeFromCaddyfile(text, original))).toBe("renamed");
  });
  it.each([
    (text: string) => text + "extra.example.net { respond ok }\n",
    (text: string) => text.replace("encode gzip", "respond ok"),
    (text: string) => text.replace("encode gzip", "import extra"),
    (text: string) => text.replace("encode gzip", "encode zstd"),
    (text: string) => text.replace("# docklet-route: example-app", "# docklet-route: bad name"),
    (text: string) => text.replace("# docklet-route: example-app\n", ""),
    (text: string) => text.replace("media.example.com {", "media.example.com, other.example.com {"),
    (text: string) => text.replace("reverse_proxy", "reverse_proxy @unknown"),
    (text: string) => text.replace("encode gzip", "encode gzip\n encode gzip"),
    (text: string) => text.replace(original.url, original.url + "/base"),
    (text: string) => text + "# unexpected metadata\n",
  ])("rejects text outside the form grammar", (change) => {
    expect(() => routeFromCaddyfile(change(routeCaddyfile(original)), original)).toThrow();
  });
  it("rejects DNS/SAN mismatches, unknown middlewares and unsupported upstream shapes", () => {
    expect(() => routeFromCaddyfile(routeCaddyfile({ ...dns, certificateDomain: "example.net" }), original)).toThrow();
    expect(() => routeCaddyfile({ ...original, middlewares: ["unknown"] })).toThrow("middlewareUnsupported");
    expect(caddyProblem({ ...original, url: "http://service:80/base" })).toBe("caddyValidation");
    expect(caddyProblem({ ...original, path: "/api { respond ok }" })).toBe("caddyValidation");
    expect(proxyStore.get().instances.find((p) => p.id === "caddy-example")?.kind).toBe("caddy");
  });
});
