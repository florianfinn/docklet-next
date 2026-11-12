// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { describe, expect, it } from "vitest";
import { parse } from "yaml";
import { coversDomain, INITIAL_ROUTES, routeFromYaml, routeYaml, validDomain, wildcardProblem, proxyStore, routeName, migrateRouteNames, routeNameConflict, duplicateRoute } from "./model";

describe("wildcard route and certificate coverage", () => {
  it("keeps automatic TLS with the lab's empty default resolver distinct from imported TLS", () => {
    const route = { ...INITIAL_ROUTES[0]!, resolver: "" };
    expect(routeFromYaml(routeYaml(route), route).tls).toBe("auto");
    expect(routeFromYaml(routeYaml({ ...route, tls: "custom" }), route).tls).toBe("custom");
  });
  it("matches exactly one subdomain level and emits explicit ACME domains", () => {
    const route = { ...INITIAL_ROUTES[0]!, domain: "*.example.com", path: "/api" };
    const text = routeYaml(route);
    const router = parse(text).http.routers[routeName(route)];
    const expression = router.rule.match(/HostRegexp\(`([^`]+)`\)/)[1];
    const matches = new RegExp(expression);
    expect(matches.test("app.example.com")).toBe(true);
    for (const name of ["example.com", "a.b.example.com", "app.exampleXcom", "app.example.com.evil.test"]) expect(matches.test(name)).toBe(false);
    expect(router.tls.domains).toEqual([{ main: "example.com", sans: ["*.example.com"] }]);
    expect(routeFromYaml(text, route)).toMatchObject({ domain: "*.example.com", path: "/api", certificateDomain: "example.com" });
  });
  it("keeps an exact route separate from its wildcard SSL request", () => {
    const route = { ...INITIAL_ROUTES[0]!, certificateDomain: "example.com" };
    const router = parse(routeYaml(route)).http.routers[routeName(route)];
    expect(router.rule).toBe("Host(`media.example.com`)");
    expect(router.tls.domains[0].sans).toEqual(["*.example.com"]);
    expect(routeFromYaml(routeYaml(route), route).domain).toBe("media.example.com");
  });
  it("does not offer a single-host certificate as wildcard coverage", () => {
    expect(coversDomain(["app.example.com"], "*.example.com")).toBe(false);
    expect(coversDomain(["*.example.com"], "example.com")).toBe(false);
    expect(coversDomain(["*.example.com"], "a.b.example.com")).toBe(false);
    expect(coversDomain(["*.example.com"], "*.example.com")).toBe(true);
    expect(coversDomain(["*.example.com"], "app.example.com")).toBe(true);
  });
  it("checks wildcard issuance against the same proxy and a covering DNS resolver", () => {
    const route = { ...INITIAL_ROUTES[0]!, domain: "*.example.com" };
    const resolvers = proxyStore.get().resolvers;
    expect(wildcardProblem(route, resolvers)).toBeUndefined();
    expect(wildcardProblem({ ...route, proxy: "boreal", resolver: "http-primary" }, resolvers)).toBe("dnsRequired");
    expect(wildcardProblem({ ...route, certificateDomain: "example.net" }, resolvers)).toBe("coverageMismatch");
    expect(wildcardProblem({ ...route, tls: "custom" }, resolvers)).toBeUndefined();
  });
  it.each(["*.*.example.com", "app.*.example.com", "*.example..com", "*.example.com/path", "*.example.com`)"])("rejects unsupported wildcard domain %s", (domain) => {
    expect(validDomain(domain)).toBe(false);
  });
});

describe("readable configuration names", () => {
  it("uses the editable name for the router, reference and service without exposing internal IDs", () => {
    const route = { ...INITIAL_ROUTES[0]!, id: "790c9dc9-bd51-4b7a-af65-bc146b412345", name: "media-proxy" };
    const text = routeYaml(route);
    const doc = parse(text);
    expect(Object.keys(doc.http.routers)).toEqual(["media-proxy"]);
    expect(doc.http.routers["media-proxy"].service).toBe("media-proxy");
    expect(doc.http.services["media-proxy"].loadBalancer.servers[0].url).toBe("http://jellyfin:8096");
    expect(text.includes(route.id)).toBe(false);
    expect(text.includes(route.containerId!)).toBe(false);
    expect(routeFromYaml(text, route).id).toBe(route.id);
  });
  it("accepts a coordinated YAML rename while keeping the Docklet identity and backend", () => {
    const original = { ...INITIAL_ROUTES[0]!, name: "old-route" };
    const text = routeYaml(original).replaceAll("old-route", "media-public");
    expect(routeFromYaml(text, original)).toMatchObject({ id: original.id, name: "media-public", url: original.url });
  });
});

describe("route name identity and migration", () => {
  it("assigns unique readable names to legacy routes without changing IDs or destinations", () => {
    const legacy = [INITIAL_ROUTES[0]!, { ...INITIAL_ROUTES[0]!, id: "another-route-id" }];
    const named = migrateRouteNames(legacy);
    expect(named.map((r) => r.name)).toEqual(["media-example-com", "media-example-com-2"]);
    expect(named.map((r) => [r.id, r.url, r.containerId])).toEqual(legacy.map((r) => [r.id, r.url, r.containerId]));
    expect(migrateRouteNames(named)).toEqual(named);
    const renamedDomain = { ...named[0]!, domain: "other.example.com" };
    expect(routeName(renamedDomain)).toBe("media-example-com");
  });
  it("keeps configuration stable when only the container ID changes", () => {
    const route = { ...INITIAL_ROUTES[0]!, name: "media" };
    expect(routeYaml({ ...route, containerId: "recreated-container" })).toBe(routeYaml(route));
  });
  it("prevents collisions per instance and gives duplicates a separate name and identity", () => {
    const route = { ...INITIAL_ROUTES[0]!, name: "media" };
    const copy = duplicateRoute(route, [route]);
    expect(copy.name).toBe("media-2");
    expect(copy.id === route.id).toBe(false);
    expect(copy.url).toBe(route.url);
    expect(routeNameConflict(copy, [route])).toBe(false);
    expect(routeNameConflict({ ...copy, name: "media" }, [route])).toBe(true);
    expect(routeNameConflict({ ...copy, name: "media", proxy: "boreal" }, [route])).toBe(false);
    expect(routeNameConflict(route, [route])).toBe(false);
  });
  it("accepts legacy ID-keyed YAML and emits the readable name afterwards", () => {
    const route = { ...INITIAL_ROUTES[0]!, id: "790c9dc9-bd51-4b7a-af65-bc146b412345", name: "media-public" };
    const parsed = routeFromYaml(routeYaml(route).replaceAll("media-public", route.id), route);
    expect(parsed.id).toBe(route.id);
    expect(parsed.name).toBe("media-public");
    expect(routeYaml(parsed).includes(route.id)).toBe(false);
  });
  it("rejects ambiguous documents and broken router-to-service references", () => {
    const route = { ...INITIAL_ROUTES[0]!, name: "media-public" };
    const text = routeYaml(route);
    expect(() => routeFromYaml(text.replace("service: media-public", "service: missing"), route)).toThrow();
    const doc = parse(text);
    doc.http.routers.extra = doc.http.routers["media-public"];
    expect(() => routeFromYaml(JSON.stringify(doc), route)).toThrow();
  });
});
