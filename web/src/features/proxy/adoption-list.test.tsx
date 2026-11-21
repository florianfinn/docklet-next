// Source: docklet-next/issues/549
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original overview structure, production boundary and proven import ownership assertions.

import type { ReactNode } from "react";
import { readFileSync } from "node:fs";
import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import type { ContainerInfo, Stack } from "@/domain/model";
import { inventoryId, mapContainer, mapStack, type ProjectRow } from "@/data/http/inventory-mapping";
import { RouteList } from "./RouteList";
import { ManualImportHint } from "./ManualImportHint";
import { manualImportStack, simpleHost, usedDomains } from "./suggestions";
import { instance, Providers, route } from "./proxy.test-support";

const data = vi.hoisted(() => ({ containers: [] as ContainerInfo[], stacks: [] as Stack[], narrow: false }));
vi.mock("@/data/queries", async importOriginal => ({
  ...await importOriginal<typeof import("@/data/queries")>(),
  useContainers: () => ({ data: data.containers }), useStacks: () => ({ data: data.stacks }),
}));
vi.mock("@/shared/hooks/use-media-query", () => ({ useMediaQuery: () => data.narrow }));
vi.mock("./CertificateCheck", () => ({ CertificateCheck: ({ frame }: { frame?: (content: ReactNode) => ReactNode }) => { const content = <section aria-label="Served certificate">Observed certificate details</section>; return frame ? frame(content) : content; } }));
beforeEach(() => { data.containers = []; data.stacks = []; data.narrow = false; });
function showList(rules = [route]) {
  return render(<Providers><RouteList instance={instance} routes={rules} hashes={{ [route.file]: "a".repeat(64) }} writable edit={vi.fn()} action={vi.fn()} /></Providers>);
}
it("row 6: ignores non-simple rules for suggestions and renders them unchanged", () => {
  const compound = "Host(`compound.example.com`) && PathPrefix(`/api`)", regexp = "HostRegexp(`.+example.com`)";
  expect(simpleHost(compound)).toBe(undefined); expect(simpleHost(regexp)).toBe(undefined);
  expect(usedDomains([route, { ...route, id: "compound", rule: compound }, { ...route, id: "regexp", rule: regexp }])).toEqual(["app.example.com"]);
  showList([{ ...route, rule: compound }, { ...route, id: "regexp", rule: regexp }]);
  expect(screen.queryByRole("heading", { name: compound }) !== null).toBe(true);
  expect(screen.queryByRole("heading", { name: regexp }) !== null).toBe(true);
});
it("row 7: production proxy sources have no imports or strings from the design lab", () => {
  const sources = import.meta.glob(["./*.tsx", "./*.ts", "!./*.test.tsx", "!./*.test.ts", "!./*.test-support.ts", "!./*.test-support.tsx"], { query: "?raw", import: "default", eager: true });
  const offenders = Object.entries(sources).filter(([, source]) => /(?:from\s*|import\s*(?:\(\s*)?)["'][^"']*design-lab|proxyLab/.test(String(source))).map(([path]) => path);
  expect(offenders).toEqual([]);
  const guard = readFileSync(`${process.cwd()}/build/production-boundary.ts`, "utf8");
  expect(guard.includes("features\\/design-lab\\/")).toBe(true);
});
it("row 8: expanded certificate details occupy a full-width companion table row", () => {
  data.stacks = [stack()];
  showList(); const certificate = screen.getByRole("region", { name: "Served certificate" });
  const cell = certificate.closest("td")!;
  expect(cell.colSpan).toBe(6);
  expect(cell.parentElement?.hasAttribute("data-certificate-row")).toBe(true);
  expect(cell.parentElement?.previousElementSibling?.querySelector("h3")?.textContent).toBe("app.example.com");
  expect(cell.parentElement?.children.length).toBe(1);
});
it("uses overview card rows below md and supports search by stack", () => {
  data.narrow = true;
  data.stacks = [{ id: inventoryId(instance.environmentId, "project"), name: "example-stack" } as Stack];
  showList([{ ...route, projectId: "project" }]);
  expect(screen.getByRole("list", { name: "Routes" }).querySelectorAll("[data-overview-card]").length).toBe(1);
  fireEvent.change(screen.getByRole("textbox", { name: "Search routes" }), { target: { value: "example-stack" } });
  expect(screen.queryByRole("heading", { name: "app.example.com" }) !== null).toBe(true);
  fireEvent.change(screen.getByRole("textbox", { name: "Search routes" }), { target: { value: "absent" } });
  expect(screen.queryByRole("heading", { name: "app.example.com" }) === null).toBe(true);
});
const observation = { stale: false, observedAt: "2026-11-21T00:00:00Z" };
const proxyContainer = (patch: Partial<ContainerInfo> = {}) => ({
  ...mapContainer({ ...{ autoUpdateEnabled: false, command: "", created: 0, hostConfig: {}, imageId: "example-image", imagePlatform: "unknown", mounts: [], originSource: "detected" as const, ports: [], networkSettings: { networks: {} }, labels: {}, origin: "standalone" as const }, id: instance.identity.containerId, names: ["/example-proxy"], state: "running", status: "Up", image: "caddy:2.10.2", origin: "compose", labels: { "com.docker.compose.project": "example", "com.docker.compose.project.working_dir": "/srv/projects/example" } }, instance.environmentId, [], observation),
  stackId: "display-owner-is-not-authority", ...patch,
});
const stack = (patch: Partial<Stack> = {}) => ({
  ...mapStack({ id: "proven-stack", name: "example", status: "running", path: "/srv/projects/example" } as ProjectRow, instance.environmentId, [], observation), ...patch,
});
it("row 9: manual import links only from unique same-host Compose path evidence, including single-file Caddy", () => {
  data.containers = [proxyContainer(), proxyContainer({ hostId: "host-two", id: inventoryId("host-two", instance.identity.containerId), stackId: "wrong-host" })];
  data.stacks = [stack(), stack({ id: "same-name", path: "/srv/projects/other" }), stack({ id: "other-host", hostId: "host-two" })];
  const i = { ...instance, kind: "caddy" as const, source: { ...instance.source, kind: "single-file" as const, projectPath: undefined } };
  render(<Providers><ManualImportHint instance={i} /></Providers>);
  expect(screen.queryByRole("link", { name: "Open stack editor" }) !== null).toBe(true);
  expect(screen.getByRole("link", { name: "Open stack editor" }).getAttribute("href")).toBe("/stacks/host-one~proven-stack?tab=editor");
});
it.each(["duplicate", "no-path", "no-match", "other-host", "wrong-id"])("row 9: %s ownership stays plain text", kind => {
  data.containers = [proxyContainer(kind === "no-path" ? { dockerLabels: {} } : kind === "other-host" ? { hostId: "host-two", id: inventoryId("host-two", instance.identity.containerId) } : kind === "wrong-id" ? { id: "raw-container-id" } : {})];
  data.stacks = kind === "duplicate" ? [stack(), stack({ id: "duplicate" })] : [stack(kind === "no-match" ? { path: "/srv/projects/other" } : {})];
  expect(manualImportStack(instance, data.containers, data.stacks)).toBe(undefined);
  render(<Providers><ManualImportHint instance={instance} /></Providers>);
  expect(screen.queryByRole("link") === null).toBe(true);
  expect(screen.queryByText(/Add this exact line/) !== null).toBe(true);
});

it("S3 R1 N4: entry-point TLS shows a closed lock instead of reading as no TLS", () => {
  const entryPoints = [{ name: "web", address: ":80", tls: false }, { name: "secure", address: ":443", tls: true }];
  const plain = { ...route, id: "plain", tls: false, resolver: undefined, entryPoints: ["web"] };
  const viaEntry = { ...route, id: "entry", tls: false, resolver: undefined, entryPoints: ["secure"] };
  const viaDefaults = { ...route, id: "defaults", tls: false, resolver: undefined, entryPoints: [] };
  render(<Providers><RouteList instance={instance} routes={[plain, viaEntry, viaDefaults]} entryPoints={entryPoints} writable edit={vi.fn()} action={vi.fn()} /></Providers>);
  expect(screen.getAllByText("TLS from entry point").length).toBe(1);
  expect(screen.getAllByText("Default entry points may serve TLS").length).toBe(1);
  expect(screen.getAllByText("No route certificate").length).toBe(1);
});
it("S3 R1 N4 N7: label routes get a readable summary and infrastructure routes an Internal source", () => {
  const label = { ...route, id: "label", class: "label-read-only" as const, service: "whoami", containerName: "example-whoami-1", projectId: "project", references: [] };
  const infra = { ...route, id: "infra", class: "infrastructure" as const };
  render(<Providers><RouteList instance={instance} routes={[label, infra]} writable edit={vi.fn()} action={vi.fn()} /></Providers>);
  for (const term of ["Match", "Target", "TLS", "Middlewares"]) expect(screen.queryByText(term, { selector: "dt" }) !== null).toBe(true);
  expect(screen.queryByText("whoami · example-whoami-1") !== null).toBe(true);
  expect(screen.queryByRole("link", { name: "Open stack editor" })?.getAttribute("href")).toContain("?tab=editor");
  expect(screen.queryByText("Internal") !== null).toBe(true);
});
