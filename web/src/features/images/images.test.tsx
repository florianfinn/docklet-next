import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { errorCode } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderRun } from "@/data/message-code";
import { simulationStore } from "@/data/demo/simulation";
import { imageUsage, parseImageReference, pruneCandidates, type DockerImage } from "@/domain/images";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderRoutes } from "@/test/render";

import ImagesPage from "./ImagesPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false, actionSpeed: 5 }));
});

afterEach(() => {
  vi.useRealTimers();
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

/** Lets the simulated action runner finish without waiting in real time. */
async function settle(adapter: ReturnType<typeof createDemoAdapter>, id: string) {
  for (let i = 0; i < 40; i++) {
    const run = (await adapter.listActions()).find((r) => r.id === id);
    if (run && run.status !== "running") return renderRun(run, "de");
    await vi.advanceTimersByTimeAsync(500);
  }
  throw new Error("run did not settle");
}

describe("image references", () => {
  it("accepts Docker references with registry, path and tag and refuses digests and uppercase", () => {
    expect(parseImageReference("redis")).toEqual({ repository: "redis", tag: "latest" });
    expect(parseImageReference("ghcr.io/example/app:1.2.0")).toEqual({ repository: "ghcr.io/example/app", tag: "1.2.0" });
    expect(parseImageReference("registry.example.com:5000/team/tool:v2")).toEqual({ repository: "registry.example.com:5000/team/tool", tag: "v2" });
    expect(["", "UPPER/case", "redis@sha256:abc", "bad tag", "a::b"].map(parseImageReference)).toEqual([null, null, null, null, null]);
  });

  it("never offers an image with a consumer, running or stopped, for removal", () => {
    const base: DockerImage = { id: "i", hostId: "h", repository: "r", tag: "1", aliases: [], digest: "sha256:0", sizeMb: 1, createdAt: "", architecture: "amd64", os: "linux", layers: 1, consumers: [] };
    const stopped = { ...base, id: "a", consumers: [{ containerId: "c", name: "c", hostId: "h", state: "stopped" as const }] };
    const unused = { ...base, id: "b" };
    const dangling = { ...base, id: "d", tag: null };
    expect([imageUsage(stopped), imageUsage(unused), imageUsage(dangling)]).toEqual(["in-use", "unused", "dangling"]);
    expect(pruneCandidates([stopped, unused, dangling], "dangling").map((i) => i.id)).toEqual(["d"]);
    expect(pruneCandidates([stopped, unused, dangling], "unused").map((i) => i.id)).toEqual(["b", "d"]);
  });
});

describe("demo images adapter", () => {
  it("derives consumers from containers, including the agent, and follows an update to the new image", async () => {
    vi.useFakeTimers();
    const adapter = createDemoAdapter();
    const images = await adapter.listImages();
    const jellyfin = images.find((i) => i.hostId === "atlas" && i.repository === "jellyfin/jellyfin" && i.tag === "10.10.3");
    expect(jellyfin?.consumers.map((c) => c.name)).toEqual(["media-jellyfin-1"]);
    expect(images.filter((i) => i.consumers.some((c) => c.system)).length).toBeGreaterThan(0);
    // Installing the update moves the consumer to the new image; the old one stays as unused until pruned.
    const run = await adapter.installStackUpdates("media", ["jellyfin"]);
    await settle(adapter, run);
    const after = await adapter.listImages();
    const old = after.find((i) => i.id === jellyfin?.id);
    const next = after.find((i) => i.hostId === "atlas" && i.repository === "jellyfin/jellyfin" && i.tag !== "10.10.3" && i.consumers.length);
    expect([old && imageUsage(old), next?.consumers[0]?.name]).toEqual(["unused", "media-jellyfin-1"]);
  });

  it("refuses to remove an image in use or on a disconnected host and removes an unused one", async () => {
    const adapter = createDemoAdapter();
    const images = await adapter.listImages();
    const used = images.find((i) => i.hostId === "atlas" && i.consumers.length && !i.consumers.some((c) => c.system))!;
    const offline = images.find((i) => i.hostId === "cedar")!;
    const unused = images.find((i) => i.hostId === "atlas" && imageUsage(i) === "unused")!;
    expect(await adapter.removeImage(used.id).catch(errorCode)).toBe("in-use");
    expect(await adapter.removeImage(offline.id).catch(errorCode)).toBe("host-offline");
    await adapter.removeImage(unused.id);
    expect((await adapter.listImages()).some((i) => i.id === unused.id)).toBe(false);
  });

  it("pulls a new reference with simulated layer output and prunes only images without consumers", async () => {
    vi.useFakeTimers();
    const adapter = createDemoAdapter();
    expect(await adapter.pullImage({ hostId: "delta", reference: "NOT valid" }).catch(errorCode)).toBe("invalid-name");
    const run = await settle(adapter, await adapter.pullImage({ hostId: "delta", reference: "ghcr.io/example/app:1.2.0" }));
    expect(run.status).toBe("succeeded");
    expect(run.output.some((l) => l.stream === "cmd" && l.text === "docker pull ghcr.io/example/app:1.2.0") && run.output.some((l) => /Pull complete/.test(l.text))).toBe(true);
    const pulled = (await adapter.listImages()).find((i) => i.hostId === "delta" && i.repository === "ghcr.io/example/app");
    expect([pulled?.tag, pulled?.version, pulled?.architecture]).toEqual(["1.2.0", "1.2.0", "arm64"]);

    const before = await adapter.listImages();
    const inUse = before.filter((i) => i.consumers.length).map((i) => i.id);
    const prune = await adapter.pruneImages({ hostIds: ["atlas", "boreal", "cedar", "delta"], mode: "unused" });
    await settle(adapter, prune!);
    const after = await adapter.listImages();
    // Every image with a consumer survives; unused ones on connected hosts are gone; the disconnected host is untouched.
    expect(inUse.every((id) => after.some((i) => i.id === id))).toBe(true);
    expect(after.filter((i) => i.hostId !== "cedar" && imageUsage(i) !== "in-use")).toEqual([]);
    expect(after.some((i) => i.hostId === "cedar" && imageUsage(i) === "unused")).toBe(true);
    expect(await adapter.pruneImages({ hostIds: ["atlas"], mode: "dangling" })).toBe(null);
  });
});

describe("images page", () => {
  it("filters by usage from the URL, opens details and blocks removing images in use", async () => {
    renderRoutes([{ path: "/images", element: <ImagesPage /> }], ["/images?usage=dangling"]);
    const rows = await screen.findAllByRole("button", { name: /^Details zu .+ öffnen$/ });
    expect(rows.length).toBe(2);
    await userEvent.click(screen.getByRole("radio", { name: "In Nutzung" }));
    await userEvent.click(screen.getByRole("button", { name: "Details zu jellyfin/jellyfin:10.10.3 öffnen" }));
    const sheet = await screen.findByRole("dialog");
    expect(within(sheet).getByText("media-jellyfin-1") !== null && within(sheet).getByText(/^sha256:[0-9a-f]{12}…$/) !== null).toBe(true);
    expect((within(sheet).getByRole("button", { name: "Entfernen" }) as HTMLButtonElement).disabled).toBe(true);
    // Versions are shown as people read them in the list; digests never appear there.
    expect(screen.getAllByText("10.10.3").length).toBeGreaterThan(0);
  });

  it("previews a prune with what goes and what stays", async () => {
    renderRoutes([{ path: "/images", element: <ImagesPage /> }], ["/images"]);
    await screen.findAllByRole("button", { name: /^Details zu .+ öffnen$/ });
    await userEvent.click(screen.getByRole("button", { name: "Aufräumen" }));
    const dialog = await screen.findByRole("dialog", { name: "Images aufräumen" });
    await waitFor(() => expect(within(dialog).getByText(/^2 Images werden entfernt/) !== null).toBe(true));
    expect(within(dialog).getByText(/bleiben erhalten/) !== null && within(dialog).getByText(/übersprungen: Cedar/) !== null).toBe(true);
  });
});
