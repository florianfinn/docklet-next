// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { describe, expect, it } from "vitest";
import { imageRegistryFixture, mutations, registryDto, registryDraft } from "./image-registry.test-support";
import { mapRegistry, registryWrite } from "./registry-commands";
import { registryHost, registryReference } from "@/domain/registries";
import { mapImageObservation } from "./image-mapping";
import { imageDto } from "./image-registry.test-support";
import { imageRemovalBlocked, imageUsage, pruneCandidates, type DockerImage } from "@/domain/images";

describe("registry mapping", () => {
  it("whitelists the public projection, dropping all credential identities and secrets", async () => {
    const f = imageRegistryFixture(); f.state.registries = [{ ...registryDto, username: "hidden-identity", token: crypto.randomUUID(), awsAccessKeyId: crypto.randomUUID(), awsSecretAccessKey: crypto.randomUUID() }];
    expect(await f.adapter.listContainerRegistries()).toEqual([{ id: "saved", url: registryDto.url, description: "Example", registryType: "generic", enabled: true, insecure: false, hasCredential: true, repositoryNames: [], awsRegion: undefined }]);
    expect(() => mapRegistry({ ...registryDto, url: "https://identity:secret@registry.example.com" })).toThrow();
  });
  it("omits blank replacements and clears the complete credential explicitly", async () => {
    const f = imageRegistryFixture();
    await f.adapter.updateContainerRegistry("saved", { ...registryDraft, username: "", token: "" });
    expect(mutations(f)[0]?.body).toEqual({ ...registryDraft });
    await f.adapter.updateContainerRegistry("saved", { ...registryDraft, clearCredential: true });
    expect(mutations(f)[1]?.body).toEqual({ ...registryDraft, clearCredential: true });
    expect(registryWrite({ ...registryDraft, clearCredential: true, token: crypto.randomUUID(), username: "ignored" })).not.toHaveProperty("token");
  });
  it("requires the clear credential capability independently", async () => {
    const f = imageRegistryFixture(); f.state.denied.add("clearContainerRegistryCredential:saved");
    await expect(f.adapter.updateContainerRegistry("saved", { ...registryDraft, clearCredential: true })).rejects.toMatchObject({ outcome: { kind: "not-available" } });
    expect(mutations(f)).toEqual([]);
  });
  it.each(["listContainerRegistries", "createContainerRegistry", "updateContainerRegistry", "deleteContainerRegistry", "testContainerRegistry"] as const)("gates %s", async (operation) => {
    const f = imageRegistryFixture(); f.state.denied.add(`${operation}:${["listContainerRegistries", "createContainerRegistry"].includes(operation) ? "0" : "saved"}`);
    const call = () => operation === "listContainerRegistries" ? f.adapter.listContainerRegistries() : operation === "createContainerRegistry" ? f.adapter.createContainerRegistry(registryDraft) : operation === "updateContainerRegistry" ? f.adapter.updateContainerRegistry("saved", registryDraft) : operation === "deleteContainerRegistry" ? f.adapter.deleteContainerRegistry("saved") : f.adapter.testContainerRegistry("saved");
    await expect(call()).rejects.toMatchObject({ outcome: { kind: "not-available" } }); expect(mutations(f)).toEqual([]);
  });
  it.each(["registry.authentication-failed", "registry.authentication-required", "registry.unavailable", "registry.rate-limited"])("reads flat typed registry test %s without expiring the session", async (code) => {
    const f = imageRegistryFixture(); f.state.response = { status: code === "registry.rate-limited" ? 429 : 502, body: { ok: false, code, checkedAt: "2026-10-25T00:00:00Z", message: "untrusted" } };
    expect(await f.adapter.testContainerRegistry("saved")).toEqual({ ok: false, code, checkedAt: "2026-10-25T00:00:00Z" });
    expect((await f.adapter.getCapability("listImages")).available).toBe(true);
  });
  it("keeps nested CRUD and top-level probe formats distinct", async () => {
    const f = imageRegistryFixture(); f.state.response = { status: 409, body: { success: false, error: { code: "registry.exists", message: "untrusted" } } };
    await expect(f.adapter.createContainerRegistry(registryDraft)).rejects.toMatchObject({ outcome: { code: "registry.exists", status: 409 } });
  });
  it("does not retry uncertain writes", async () => {
    const f = imageRegistryFixture(); f.state.failTransport = true;
    await expect(f.adapter.createContainerRegistry(registryDraft)).rejects.toMatchObject({ outcome: { code: "registry.outcome-unknown" } });
    expect(mutations(f)).toHaveLength(1);
  });
  it("selects saved credential lookup by reference host, including Docker Hub", () => {
    expect(registryHost("https://registry.example.com:5000")).toBe("registry.example.com:5000");
    expect(registryReference("other.example.com/team/app:1", "registry.example.com")).toBe("registry.example.com/team/app:1");
    expect(registryReference("redis:7", registryHost("https://index.docker.io/v1/"))).toBe("docker.io/library/redis:7");
  });
});
describe("read-only image observations", () => {
  it.each(["version", "rebuild", "unknown", "error", "unchecked", "local", "current"] as const)("preserves %s independently of legacy hasUpdate", (state) => {
    const update = { state, checkedAt: "2026-10-25T00:00:00Z", currentVersion: "1", latestVersion: "2", hasUpdate: false, authState: "anonymous" as const, currentDigest: "", latestDigest: "", checkTime: "", error: "never rendered", responseTimeMs: 0, imageRef: "example:1", updateType: "" };
    expect(mapImageObservation({ ...imageDto, updateInfo: update }).updateObservation).toEqual({ state, checkedAt: update.checkedAt, currentVersion: "1", candidateVersion: "2", code: undefined });
    expect(JSON.stringify(mapImageObservation({ ...imageDto, updateInfo: update }))).not.toContain("never rendered");
  });
  it("never treats missing usage proof or protection as removable", () => {
    const base: DockerImage = { id: "host~image", hostId: "host", repository: "example", tag: "1", aliases: [], digest: "", createdAt: "", sizeMb: 1, architecture: "", os: "", layers: 0, consumers: [] };
    const unknown = { ...base, ...mapImageObservation(imageDto), usageState: "unknown" as const };
    expect(imageUsage(unknown)).toBe("unknown"); expect(imageRemovalBlocked(unknown)).toBe(true);
    for (const reason of ["system", "restore-point", "active-update", "in-use"]) {
      const protectedImage = { ...base, protectionReasons: [{ reason }] };
      expect(imageRemovalBlocked(protectedImage)).toBe(true); expect(pruneCandidates([protectedImage], "unused")).toEqual([]);
    }
  });
});
it("drops registry test and write results from a replaced identity", async () => {
  const { createRegistryCommands } = await import("./registry-commands");
  let generation = 0, release!: (response: Response) => void;
  const { vi } = await import("vitest");
  const fetcher = vi.fn(() => new Promise<Response>((resolve) => { release = resolve; })), changed = vi.fn();
  const commands = createRegistryCommands({ baseUrl: "/api", client: { fetch: fetcher as typeof fetch, token: async () => "example" }, generation: () => generation, changed,
    capability: async (operationId, resourceRef) => ({ available: true, operationId, resourceRef }) });
  const pending = commands.updateContainerRegistry("saved", registryDraft);
  await vi.waitFor(() => expect(fetcher).toHaveBeenCalledOnce());
  generation++;
  release(new Response(JSON.stringify({ success: true, data: registryDto })));
  await expect(pending).rejects.toMatchObject({ revert: undefined }); expect(changed).not.toHaveBeenCalled();
});

it("treats malformed usage/protection evidence as unknown and preserves server in-use proof", () => {
  const bad = { ...imageDto, usageState: "invalid" } as unknown as typeof imageDto;
  expect(mapImageObservation(bad).usageState).toBe("unknown");
  expect(mapImageObservation({ ...imageDto, protectionReasons: {} } as unknown as typeof imageDto).usageState).toBe("unknown");
  expect(mapImageObservation({ ...imageDto, inUse: true }).protectionReasons).toContainEqual({ reason: "in-use" });
});
