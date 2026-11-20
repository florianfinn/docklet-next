// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

import { afterEach, expect, it } from "vitest";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { MemoryRouter } from "react-router";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { queryKeys } from "@/data/query-keys";
import { IntlProvider } from "use-intl";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterContext } from "@/data/use-adapter";
import { RuntimePeerDialog } from "@/shared/components/runtime-peer-dialog";
import { runtimeBlocker } from "../runtime-blocker";
import { runtimePeerAudit } from "./runtime-peer-audit";
import { renderMessage } from "../message-code";
import { envelope, response, RUN_ID, runtimeFixture, snapshot } from "./runtime-commands.test-support";
import producerFixture from "./runtime-peer.fixture.json";

afterEach(cleanup);
function dialog(f: ReturnType<typeof runtimeFixture>, locale: "en-US" | "de" = "en-US") {
 const client = new QueryClient({ defaultOptions: { queries: { staleTime: Infinity, retry: false } } });
 client.setQueryData(queryKeys.hosts, [{ id: "direct", name: "Example host" }]);
 return render(<QueryClientProvider client={client}><AdapterContext.Provider value={f.adapter}><IntlProvider locale={locale} messages={catalogs[locale]}><MemoryRouter><RuntimePeerDialog /></MemoryRouter></IntlProvider></AdapterContext.Provider></QueryClientProvider>);
}
function refused(blocker: unknown = producerFixture, status = 409, data?: unknown) { return response({ success: false, error: { code: "runtime.target-unavailable", blocker }, ...(data ? { data } : {}) }, status); }
function admission(f: ReturnType<typeof runtimeFixture>) { return envelope({ activityId: RUN_ID, code: "run.started", activity: f.state.run }, 202); }

it.each(["en-US", "de"] as const)("cancel of the real typed %s dialog sends no additional command", async (locale) => {
 const f = runtimeFixture(); dialog(f, locale); f.state.command = () => f.commands().length === 1 ? refused() : admission(f);
 const task = f.helper.runStackCommand("0~target", "restart").catch((error: unknown) => error);
 expect((await screen.findByRole("alertdialog")).textContent).toContain("peer");
 expect(screen.getByRole("alertdialog").getAttribute("data-slot")).toBe("alert-dialog-content");
 expect(screen.getByRole("alertdialog").textContent).toContain(locale === "de" ? "Compose-Identität" : "Compose identity");
 fireEvent.click(screen.getByRole("button", { name: locale === "de" ? "Abbrechen" : "Cancel" }));
 await task; expect(f.commands()).toHaveLength(1);
 f.helper.reset();
});

it.each(["restart", "stop", "redeploy"] as const)("confirmation sends the same %s once with the complete digest", async (command) => {
 const f = runtimeFixture(); dialog(f); f.state.run = snapshot("0", { resourceType: "project", resourceId: "target", type: "project_restart" });
 f.state.command = () => f.commands().length === 1 ? refused() : admission(f);
 const task = f.helper.runStackCommand("0~target", command);
 await screen.findByRole("alertdialog");
 const original = f.commands()[0]!;
 f.state.stopMode = "stop";
 fireEvent.click(screen.getByRole("button", { name: "Run anyway" }));
 await expect(task).resolves.toBe(RUN_ID);
 expect(f.commands()).toHaveLength(2);
 expect(f.commands()[1]!.url.toString()).toBe(original.url.toString());
 expect(JSON.parse(f.commands()[1]!.init.body as string)).toEqual({ acknowledgedUncertainty: producerFixture.uncertainty.digest });
 expect(new Headers(f.commands()[1]!.init.headers).get("Idempotency-Key")).not.toBe(new Headers(original.init.headers).get("Idempotency-Key"));
 f.helper.reset();
});

it("17 peers show sixteen entries and one more, and a new refusal requires another explicit confirmation", async () => {
 const f = runtimeFixture(); dialog(f);
 const peers = Array.from({ length: 16 }, (_,i) => ({ peerId: `example-${i}`, name: `Peer ${i}`, reason: "compose.identity-unreadable", archived: false }));
 const first = { ...producerFixture, uncertainty: { digest: "a".repeat(64), count: 17, peers } };
 const second = { ...first, uncertainty: { ...first.uncertainty, digest: "b".repeat(64), count: 18 } };
 f.state.command = () => f.commands().length === 1 ? refused(first) : f.commands().length === 2 ? refused(second) : admission(f);
 const task = f.helper.runStackCommand("0~target", "restart");
 await screen.findByText("and 1 more"); expect(screen.getAllByRole("listitem")).toHaveLength(16);
 fireEvent.click(screen.getByRole("button", { name: "Run anyway" }));
 await screen.findByText("and 2 more"); expect(f.commands()).toHaveLength(2);
 fireEvent.click(screen.getByRole("button", { name: "Run anyway" }));
 await expect(task).resolves.toBe(RUN_ID);
 expect(f.commands()).toHaveLength(3);
 expect(JSON.parse(f.commands()[1]!.init.body as string).acknowledgedUncertainty).toBe(first.uncertainty.digest);
 expect(JSON.parse(f.commands()[2]!.init.body as string).acknowledgedUncertainty).toBe(second.uncertainty.digest);
 f.helper.reset();
});

it("confirmed update preserves revision, service, verified manifest and auto restore options", async () => {
 const f = runtimeFixture(); dialog(f); f.state.run = snapshot("0", { resourceType: "project", resourceId: "target", type: "project_update" });
 f.state.command = () => f.commands().length === 1 ? refused() : admission(f);
 const input = { expectedRevision: "a".repeat(64), service: "app", expectedManifest: `sha256:${"b".repeat(64)}`, autoRestore: true };
 const task = f.helper.admitRuntimeCommand({ kind: "stack", id: "0~target", command: "update", input });
 await screen.findByRole("alertdialog"); fireEvent.click(screen.getByRole("button", { name: "Run anyway" }));
 await expect(task).resolves.toMatchObject({ runId: RUN_ID });
 expect(f.commands()).toHaveLength(2);
 expect(JSON.parse(f.commands()[1]!.init.body as string)).toEqual({ ...input, acknowledgedUncertainty: producerFixture.uncertainty.digest });
 expect(f.commands()[1]!.url.pathname).toBe(f.commands()[0]!.url.pathname);
 f.helper.reset();
});

it("hard duplicates never offer confirmation; old-agent uncertainty only offers update guidance", async () => {
 const f = runtimeFixture(); dialog(f);
 f.state.command = () => refused({ ...producerFixture, reason: "compose.duplicate", confirmable: false, uncertainty: undefined });
 await expect(f.helper.runStackCommand("0~target", "restart")).rejects.toMatchObject({ outcome: { blocker: { reason: "compose.duplicate" } } });
 expect(screen.queryByRole("alertdialog")).toBeNull(); expect(f.commands()).toHaveLength(1);
 f.state.command = () => refused({ name: "peer", reason: "compose.identity-unreadable", archived: false });
 const task = f.helper.runStackCommand("direct~target", "restart").catch((error: unknown) => error);
 await screen.findByText("Update the agent on Example host to confirm this action.");
 expect(screen.queryByRole("button", { name: "Run anyway" })).toBeNull();
 fireEvent.click(screen.getByRole("button", { name: "Cancel" })); await task; expect(f.commands()).toHaveLength(2);
 f.helper.reset();
});

it.each([
 ["recreate", (f: ReturnType<typeof runtimeFixture>) => f.helper.recreateContainer("0~web", { image: "registry.example.com/app:2" })],
 ["update", (f: ReturnType<typeof runtimeFixture>) => f.helper.installContainerUpdate("0~web", { expectedManifest: `sha256:${"b".repeat(64)}` })],
 ["update without input", (f: ReturnType<typeof runtimeFixture>) => f.helper.installContainerUpdate("0~web")],
 ["restore", (f: ReturnType<typeof runtimeFixture>) => f.helper.restoreContainerImage("0~web")],
] as const)("container %s has no acknowledgement carrier, so it stays refused without a confirm control", async (_, run) => {
 const f = runtimeFixture(); dialog(f);
 f.state.command = () => refused();
 const task = run(f).then(() => "resolved", (error: unknown) => error);
 const first = await Promise.race([task, screen.findByRole("alertdialog").then(() => "dialog")]);
 f.helper.reset(); await task;
 expect(first).not.toBe("dialog");
 expect(first).toMatchObject({ outcome: { code: "runtime.target-unavailable", blocker: { reason: "compose.name-undeterminable" } } });
 expect(f.commands()).toHaveLength(1);
});

it.each(["identity", "admitted", "server-error"] as const)("%s cannot replay a potentially executed action", async (mode) => {
 const f = runtimeFixture(); dialog(f);
 f.state.command = () => refused(producerFixture, mode === "server-error" ? 500 : 409, mode === "admitted" ? { activityId: RUN_ID } : undefined);
 const task = f.helper.runStackCommand("0~target", "restart").catch((error: unknown) => error);
 if (mode === "identity") { await screen.findByRole("alertdialog"); act(() => f.helper.reset()); } else { await task; expect(screen.queryByRole("alertdialog")).toBeNull(); }
 await task; expect(f.commands()).toHaveLength(1); f.helper.reset();
});

it("actual producer fixture is actionable while malformed confirmation fields fail closed", () => {
 expect(runtimeBlocker("runtime.target-unavailable", producerFixture)).toMatchObject({ confirmable: true, uncertainty: { count: 1 } });
 for (const patch of [{ digest: "bad" }, { count: 17 }, { peers: [{ ...producerFixture.uncertainty.peers[0], file: "../outside.env" }] }, { peers: [{ ...producerFixture.uncertainty.peers[0], peerId: "../bad" }] }, { peers: [{ ...producerFixture.uncertainty.peers[0], reason: "raw.diagnostic" }] }]) {
  expect(runtimeBlocker("runtime.target-unavailable", { ...producerFixture, uncertainty: { ...producerFixture.uncertainty, ...patch } })?.confirmable).not.toBe(true);
 }
});

it.each(["en-US", "de"] as const)("actual-use audit renders safe localized peer names, reasons and files in %s", (locale) => {
 const message = runtimePeerAudit("runtime.peer-acknowledged", { count: 17, peers: Array.from({ length: 16 }, (_,i) => ({ name: `Peer ${i}`, reason: "compose.env-file-missing", file: "service.env" })) });
 expect(message).toBeDefined();
 const text = renderMessage(message!, locale);
 expect(text).toContain("Peer 0"); expect(text).toContain("service.env"); expect(text).toContain(locale === "de" ? "und 1 weitere" : "and 1 more");
 expect(text).not.toContain("compose.env-file-missing"); expect(text).not.toContain("runtime.peer-acknowledged");
});

it("malformed audit evidence displays a localized fallback without raw diagnostics", () => {
 const message = runtimePeerAudit("runtime.peer-acknowledged", { count: 1, peers: [{ name: "/unsafe/private", reason: "raw.diagnostic" }] });
 expect(message).toBeDefined();
 expect(renderMessage(message!, "en-US")).toBe("Stack confirmation details are unavailable.");
});
