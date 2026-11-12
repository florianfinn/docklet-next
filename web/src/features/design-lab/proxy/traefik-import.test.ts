// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: a294ca3b35e942b8c194f9324fcd46791650dbf5
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { describe, expect, it } from "vitest";
import { isMap, parseDocument } from "yaml";
import { addRouteToFile, classifyTraefikFiles, isSecretFile, removeRouteFromFile, updateRouterInFile, type RouterPatch } from "./traefik-import";

const fixture = Array.from({ length: 130 }, (_, i) => "# Maintenance note " + (i + 1)).join("\n") + `
http:
  # Application routing
  routers:
    redirect:
      # HTTP catch-all
      rule: PathPrefix(\x60/\x60)
      entryPoints: [web]
      middlewares: [https]
      service: noop
    wildcard: # Requests the shared certificate
      rule: HostRegexp(\x60^[a-z0-9-]+\\.example\\.com$\x60)
      entryPoints: [websecure]
      service: wildcard
      tls:
        # Issuance is owned by Traefik
        certResolver: dns-primary # Keep the resolver annotation
        domains:
          - main: example.com # Certificate scope
            sans: ['*.example.com']
    # Exact application route
    app-one:
      rule: 'Host("app.example.com")' # Keep quoting
      entryPoints: [websecure] # Public listener
      middlewares: [auth, other@file]
      service: app-one
      tls: {} # Uses the certificate store
    app-two:
      rule: Host(\x60two.example.com\x60)
      service: shared
      tls: {}
    app-three:
      rule: Host(\x60three.example.com\x60)
      service: shared
      tls: {}
    files:
      rule: Host(\x60files.example.net\x60) && PathPrefix(\x60/files\x60)
      service: shared
      tls: {}
    priority:
      rule: Host(\x60priority.example.com\x60)
      service: priority
      priority: 100 # Deliberately outside the form
      tls: {}
    path:
      rule: Host(\x60priority.example.com\x60) && Path(\x60/exact\x60)
      service: priority
    internal:
      rule: Host(\x60internal.example.net\x60)
      entryPoints: [internal-https]
      service: internal
      tls: {}
    internal-two:
      rule: Host(\x60second.example.net\x60)
      service: internal
      tls: {}
    plugin-route:
      rule: Host(\x60plugin.example.net\x60)
      service: plugin-route
      middlewares: [security-plugin, private-auth@file, foreign@docker]
      tls: {}
  # Services are independent configuration entries
  services:
    noop:
      loadBalancer:
        servers: [{url: 'http://192.0.2.1:1'}]
    wildcard:
      loadBalancer:
        servers: [{url: 'http://192.0.2.10:8080'}]
    app-one: # Owned by one router
      loadBalancer:
        servers: [{url: 'http://192.0.2.11:8080'}]
    shared: # Shared by three routers
      loadBalancer:
        servers: [{url: 'http://192.0.2.12:8080'}]
    priority:
      loadBalancer:
        servers: [{url: 'https://[2001:db8::10]:8443'}]
    internal:
      loadBalancer:
        servers: [{url: 'http://198.51.100.10:8080'}]
    plugin-route:
      loadBalancer:
        servers: [{url: 'http://203.0.113.10:8080'}]
  # Middleware annotations are preserved as well
  middlewares:
    https:
      redirectScheme: {scheme: https, permanent: true}
    headers:
      headers: {frameDeny: true}
    auth:
      forwardAuth: {address: 'https://auth.example.net'}
    security-plugin:
      plugin: {example: {enabled: true}}
    compression:
      compress: {}
    strip:
      stripPrefix: {prefixes: [/api]}
# File footer
`;
const files = () => [
  { name: "routes.yml", text: fixture },
  { name: "other.yml", text: "http:\n  middlewares:\n    other:\n      headers: {}\n" },
  { name: "auth.secret.yml", text: "http:\n  middlewares:\n    private-auth:\n      forwardAuth: {}\n" },
];
const classify = () => classifyTraefikFiles(files());
const router = (key: string) => classify().routers.find((r) => r.key === key)!;
const readRouter = (text: string, key: string) => {
  const node = parseDocument(text).getIn(["http", "routers", key], true);
  if (!isMap(node)) throw new Error("expected-router");
  return node;
};
const comments = (text: string) => text.split("\n").filter((line) => line.includes("#")).map((line) => line.slice(line.indexOf("#")));
function expectOutsideUnchanged(before: string, after: string, key: string) {
  const a = readRouter(before, key).range!;
  expect(after.slice(0, a[0])).toBe(before.slice(0, a[0]));
  expect(after.slice(a[2] + after.length - before.length)).toBe(before.slice(a[2]));
  expect(comments(after)).toEqual(comments(before));
}

describe("Traefik import classification", () => {
  it("classifies a shared, heavily commented installation without choosing a router implicitly", () => {
    const result = classify();
    expect(result.routers).toHaveLength(11);
    expect(result.services).toHaveLength(7);
    expect(result.parseErrors).toEqual([]);
    expect(router("app-one").category).toBe("editable");
    expect(router("wildcard").category).toBe("editable");
    expect(router("redirect")).toMatchObject({ category: "infrastructure", reasons: expect.arrayContaining(["catch-all", "redirect"]) });
    expect(router("priority")).toMatchObject({ category: "textOnly", reasons: ["priority", "shared-service"] });
    expect(router("path")).toMatchObject({ category: "textOnly", reasons: ["combined-rule", "shared-service"] });
    expect(router("files")).toMatchObject({ category: "textOnly", reasons: ["shared-service"], serviceUseCount: 3 });
    expect(result.services.find((s) => s.key === "shared")?.routerCount).toBe(3);
    expect(result.entryPoints).toEqual(["internal-https", "web", "websecure"]);
    expect(result.resolvers).toEqual(["dns-primary"]);
    expect(result.middlewareTypes).toEqual(["compress", "forwardAuth", "headers", "plugin", "redirectScheme", "stripPrefix"]);
  });
  it("treats external references as valid evidence, never as an error", () => {
    const result = classify();
    expect(result.parseErrors).toEqual([]);
    expect(result.routers.find((r) => r.key === "plugin-route")).toMatchObject({ category: "editable", reasons: [], middlewares: [
      { name: "security-plugin", kind: "plugin", files: ["routes.yml"], types: ["plugin"] },
      { name: "private-auth@file", kind: "external", files: [], types: [] },
      { name: "foreign@docker", kind: "external", files: [], types: [] },
    ] });
    expect(router("app-one").middlewares.map((m) => m.kind)).toEqual(["local", "otherFile"]);
  });
  it("skips secret files by pattern before accessing text, while allowing example templates", () => {
    const names = ["auth.secret", "auth.secret.yaml", "auth.secret.toml", "nested/auth.secret.yml.backup", "AUTH.SECRET.JSON"];
    const result = classifyTraefikFiles(names.map((name) => ({ name, get text(): string { throw new Error("must-not-read"); } })).concat([
      { name: "auth.secret.yml.example", text: "http: {middlewares: {template: {headers: {}}}}" },
    ]));
    expect(result.skippedSecrets).toEqual(names);
    expect(result.parseErrors).toEqual([]);
    expect(result.middlewareTypes).toEqual(["headers"]);
    expect(isSecretFile("nested.secret/routes.yml")).toBe(false);
  });
  it("separates certificate requests, store-only TLS and no TLS", () => {
    expect(router("wildcard").certificate).toEqual({ role: "request", emptyTls: false, certResolver: "dns-primary", domains: [{ main: "example.com", sans: ["*.example.com"] }] });
    expect(router("app-one").certificate).toEqual({ role: "store", emptyTls: true });
    expect(router("redirect").certificate).toEqual({ role: "off", emptyTls: false });
    const result = classifyTraefikFiles([{ name: "routes.yml", text: fixture.replace("tls: {} # Uses", "tls: {certResolver: http-primary} # Uses") }]);
    expect(result.routers.find((r) => r.key === "app-one")?.certificate).toEqual({ role: "request", emptyTls: false, certResolver: "http-primary" });
  });
  it.each([
    ["rule: 'Host(\"app.example.com\")'", "rule: Path(\x60/exact\x60)", "path-matcher"],
    ["rule: 'Host(\"app.example.com\")'", "rule: Host(\x60app.example.com\x60) || Host(\x60other.example.net\x60)", "combined-rule"],
    ["rule: 'Host(\"app.example.com\")'", "rule: HostRegexp(\x60.*example.com\x60)", "unsupported-host-regexp"],
    ["rule: 'Host(\"app.example.com\")'", "rule: Header(\x60X-Example\x60, \x60on\x60)", "unsupported-matcher"],
    ["service: app-one", "service: missing@docker", "external-service"],
    ["service: app-one", "service: app-one\n      observability: {}", "unknown-router-key"],
    ["tls: {} # Uses", "tls: {options: strict} # Uses", "unsupported-tls"],
    ["servers: [{url: 'http://192.0.2.11:8080'}]", "servers: [{url: 'http://192.0.2.11:8080', weight: 2}]", "unsupported-service"],
  ])("retains unsupported features as text: %s → %s", (from, to, reason) => {
    const result = classifyTraefikFiles([{ name: "routes.yml", text: fixture.replace(from, to) }]);
    expect(result.parseErrors).toEqual([]);
    expect(result.routers.find((r) => r.key === "app-one")).toMatchObject({ category: "textOnly", reasons: expect.arrayContaining([reason]) });
  });
  it("reports parse failures per file and continues with readable documents", () => {
    const result = classifyTraefikFiles([...files(), { name: "broken.yml", text: "http: [" }, { name: "duplicate.yml", text: "http: {}\nhttp: {}" }]);
    expect(result.routers).toHaveLength(11);
    expect(result.parseErrors.map((e) => e.file)).toEqual(["broken.yml", "duplicate.yml"]);
    expect(result.parseErrors.every((e) => e.code && typeof e.offset === "number")).toBe(true);
  });
  it("resolves file-provider sharing across files, independent of provider suffixes", () => {
    const result = classifyTraefikFiles([...files(), { name: "extra.yml", text: "http: {routers: {extra: {rule: 'Host(`extra.example.net`)', service: 'app-one@file'}}}" }]);
    expect(result.routers.find((r) => r.key === "app-one")).toMatchObject({ category: "textOnly", serviceUseCount: 2, reasons: ["shared-service"] });
    expect(result.routers.find((r) => r.key === "extra")?.reasons).toEqual(["shared-service"]);
  });
});

describe("Traefik source-range edits", () => {
  it("preserves comments and unrelated bytes when scalar fields change", () => {
    const next = updateRouterInFile(fixture, "app-one", { rule: 'Host("new.example.com")', entryPoints: ["internal-https"], middlewares: ["headers", "other@file"], service: "replacement", tls: { certResolver: "http-primary" } });
    expectOutsideUnchanged(fixture, next, "app-one");
    expect(next).toBe(fixture.replace('Host("app.example.com")', 'Host("new.example.com")').replace("entryPoints: [websecure] # Public", "entryPoints: [internal-https] # Public").replace("middlewares: [auth, other@file]", "middlewares: [headers, other@file]").replace("service: app-one\n", "service: replacement\n").replace("tls: {} # Uses", "tls: { certResolver: http-primary } # Uses"));
    expect(readRouter(next, "app-one").toJSON()).toMatchObject({ service: "replacement", tls: { certResolver: "http-primary" } });
  });
  it("preserves nested TLS comments, original field order and all unrelated router bytes", () => {
    const next = updateRouterInFile(fixture, "wildcard", { tls: { domains: [{ sans: ["*.example.net"], main: "example.net" }], certResolver: "dns-secondary" } });
    expectOutsideUnchanged(fixture, next, "wildcard");
    expect(next).toBe(fixture.replace("certResolver: dns-primary", "certResolver: dns-secondary").replace("main: example.com", "main: example.net").replace("sans: ['*.example.com']", "sans: ['*.example.net']"));
  });
  it("round-trips every router byte-identically with a nonempty no-op patch", () => {
    for (const key of Object.keys(parseDocument(fixture).toJS().http.routers)) {
      const source = readRouter(fixture, key).toJSON();
      const patch = Object.fromEntries(Object.entries(source).filter(([k]) => ["rule", "entryPoints", "middlewares", "service", "tls"].includes(k))) as RouterPatch;
      expect(updateRouterInFile(fixture, key, patch)).toBe(fixture);
    }
  });
  it("keeps shared services on remove, including all service comments and bytes", () => {
    const next = removeRouteFromFile(fixture, "app-two");
    expect(parseDocument(next).getIn(["http", "routers", "app-two"])).toBeUndefined();
    expect(parseDocument(next).getIn(["http", "services", "shared"])).toBeDefined();
    expect(next.slice(next.indexOf("  # Services"))).toBe(fixture.slice(fixture.indexOf("  # Services")));
    expect(next).toBe(fixture.replace(/ {4}app-two:\n[\s\S]+?(?= {4}app-three:)/, ""));
    expect(classifyTraefikFiles([{ name: "routes.yml", text: next }]).services.find((s) => s.key === "shared")?.routerCount).toBe(2);
  });
  it("keeps standalone and inline comments on removal and leaves neighbouring entries intact", () => {
    const next = removeRouteFromFile(fixture, "app-one");
    expect(comments(next)).toEqual(comments(fixture));
    expect(next.slice(next.indexOf("    app-two:"))).toBe(fixture.slice(fixture.indexOf("    app-two:")));
    expect(next.slice(0, next.indexOf("    # Exact"))).toBe(fixture.slice(0, fixture.indexOf("    # Exact")));
  });
  it("adds a router and its service without rewriting pre-existing bytes", () => {
    const next = addRouteToFile(fixture, "new-route", { rule: "Host(`new.example.net`)", entryPoints: ["internal-https"], service: "new-route", tls: {} }, { key: "new-route", value: { loadBalancer: { servers: [{ url: "http://192.0.2.20:8080" }] } } });
    const doc = parseDocument(next);
    expect(doc.errors).toEqual([]);
    const addedRouter = readRouter(next, "new-route").range!;
    const serviceNode = doc.getIn(["http", "services", "new-route"], true);
    if (!isMap(serviceNode)) throw new Error("expected-service");
    const addedService = serviceNode.range!;
    const cutEntry = (text: string, range: readonly number[]) => {
      const start = text.lastIndexOf("\n", text.lastIndexOf("\n", range[0]! - 1) - 1) + 1;
      return text.slice(0, start) + text.slice(range[2]);
    };
    expect(cutEntry(cutEntry(next, addedService), addedRouter)).toBe(fixture);
    expect(comments(next)).toEqual(comments(fixture));
  });
  it("initialises missing maps and leaves the document valid after its last router is removed", () => {
    const source = "# Existing configuration\nhttp:\n  middlewares: {}\n";
    const added = addRouteToFile(source, "app-one", { rule: "Host(`app.example.com`)", service: "app-one" });
    expect(added.startsWith(source)).toBe(true);
    const removed = removeRouteFromFile(added, "app-one");
    expect(parseDocument(removed).toJS().http.routers).toEqual({});
    expect(parseDocument(removed).errors).toEqual([]);
    expect(addRouteToFile("other: {}\n", "app-one", { rule: "Host(`app.example.com`)" }).startsWith("other: {}\n")).toBe(true);
  });
  it("adds optional fields, removes optional fields and supports CRLF byte preservation", () => {
    const source = fixture.replaceAll("\n", "\r\n");
    const next = updateRouterInFile(source, "app-one", { middlewares: null, tls: null });
    expectOutsideUnchanged(source, next, "app-one");
    expect(readRouter(next, "app-one").toJSON()).not.toHaveProperty("tls");
    const added = updateRouterInFile(next, "app-one", { tls: {}, middlewares: ["headers"] });
    expect(readRouter(added, "app-one").toJSON()).toMatchObject({ tls: {}, middlewares: ["headers"] });
    expect(added.replaceAll("\r\n", "")).not.toContain("\n");
  });
  it("resizes commented block lists and removes nested TLS data without losing annotations", () => {
    const source = fixture.replace("middlewares: [auth, other@file]", "middlewares:\n        - auth # First handler\n        # Handler ordering\n        - other@file # Second handler");
    const grown = updateRouterInFile(source, "app-one", { middlewares: ["headers", "other@file", "compression"] });
    expectOutsideUnchanged(source, grown, "app-one");
    expect(readRouter(grown, "app-one").toJSON().middlewares).toEqual(["headers", "other@file", "compression"]);
    const shrunk = updateRouterInFile(source, "app-one", { middlewares: ["headers"] });
    expectOutsideUnchanged(source, shrunk, "app-one");
    expect(readRouter(shrunk, "app-one").toJSON().middlewares).toEqual(["headers"]);
    const emptied = updateRouterInFile(fixture, "wildcard", { tls: {} });
    expectOutsideUnchanged(fixture, emptied, "wildcard");
    expect(readRouter(emptied, "wildcard").toJSON().tls).toEqual({});
  });
  it("inserts a route before a newly created neighbouring services map at the same source offset", () => {
    const source = "http:\n  routers:\n    app-one:\n      rule: Host(`app.example.com`)\n      service: app-one\n";
    const next = addRouteToFile(source, "new-route", { rule: "Host(`new.example.net`)", service: "new-route" }, { key: "new-route", value: { loadBalancer: { servers: [{ url: "http://192.0.2.20:8080" }] } } });
    expect(readRouter(next, "new-route").toJSON().service).toBe("new-route");
    expect(parseDocument(next).getIn(["http", "services", "new-route"])).toBeDefined();
    expect(next.startsWith(source)).toBe(true);
  });
  it("patches and adds compact map entries without rewriting existing entries", () => {
    const source = 'http: {routers: {one: {rule: \'Host("app.example.com")\', service: one}}, services: {}}\n';
    const next = updateRouterInFile(source, "one", { service: "changed", middlewares: ["auth"] });
    expectOutsideUnchanged(source, next, "one");
    expect(next).toContain('rule: \'Host("app.example.com")\', service: changed');
    const added = addRouteToFile(source, "two", { rule: "Host(`two.example.net`)", service: "two" });
    expect(readRouter(added, "one").toJSON()).toEqual(readRouter(source, "one").toJSON());
    expect(readRouter(added, "two").toJSON().service).toBe("two");
  });
  it("rejects invalid YAML, duplicate names, indirect YAML and unsupported patch fields", () => {
    expect(() => updateRouterInFile("http: [", "app-one", {})).toThrow("invalid-yaml");
    expect(() => addRouteToFile(fixture, "app-one", {})).toThrow("duplicate-entry");
    expect(() => updateRouterInFile("http: &defaults {}\ncopy: *defaults\n", "app-one", {})).toThrow("yaml-indirection");
    expect(() => updateRouterInFile(fixture, "app-one", { priority: 1 } as RouterPatch)).toThrow("unsupported-patch-key");
  });
});
