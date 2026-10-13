import { describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { DemoDb } from "@/data/demo/demo-db";
import { sortRows } from "@/shared/lib/table-sort";

import { sortHosts } from "./hosts";
import { agentHubUrl, notificationBaseUrl, type GeneralSettings } from "./settings";
import { isSystemWorkload, withoutSystem } from "./system";
import { defaultInstallDir, hubAddress, installDirProblem } from "./validation";

describe("agent install directory", () => {
  it("allows the agent's own folder inside the project directory and blocks the root, parents and stack folders", () => {
    expect(defaultInstallDir("/home/docker/")).toBe("/home/docker/docklet-agent");
    expect(installDirProblem("/home/docker", "/home/docker/docklet-agent")).toBe(undefined);
    expect(installDirProblem("/home/docker", "/opt/docklet-agent")).toBe(undefined);
    expect(installDirProblem("/home/docker", "/home/docker/")).toBe("sameDir");
    expect(installDirProblem("/home/docker", "/home")).toBe("containsProject");
    expect(installDirProblem("/home/docker", "/home/docker/media/agent")).toBe("insideStack");
    // Prefix without a path separator is a different directory.
    expect(installDirProblem("/home/docker", "/home/dockerx")).toBe(undefined);
  });
});

describe("system components", () => {
  it("are recognised by the explicit label only, never by name", () => {
    expect(isSystemWorkload({ dockerLabels: { "io.docklet.system": "agent" } })).toBe(true);
    expect(isSystemWorkload({ dockerLabels: { "com.docker.compose.project": "docklet-agent" } })).toBe(false);
    expect(withoutSystem([{ name: "docklet-agent" }, { name: "x", dockerLabels: { "io.docklet.system": "agent" } }]).map((x) => x.name)).toEqual(["docklet-agent"]);
  });

  it("stay out of stack and container lists and counts but show up per host", async () => {
    const db = new DemoDb();
    // Delta's agent lives in its project directory, so discovery finds it as a compose project.
    expect(db.discoveredStacks().some((s) => s.hostId === "delta" && s.name === "docklet-agent")).toBe(true);
    expect(db.discoveredContainers().filter((c) => c.name === "docklet-agent").length).toBe(4);
    const adapter = createDemoAdapter();
    const [stacks, containers, system] = await Promise.all([adapter.listStacks(), adapter.listContainers(), adapter.listSystemComponents()]);
    expect(stacks.some((s) => s.name === "docklet-agent") || containers.some((c) => c.name === "docklet-agent")).toBe(false);
    expect(containers.length).toBe(24);
    expect(system.map((c) => `${c.hostId}:${c.insideProjectDir}`).sort()).toEqual(["atlas:false", "boreal:false", "cedar:false", "delta:true"]);
  });

  it("reports the installer's local check per connected host, not before", async () => {
    const hosts = await createDemoAdapter().listHosts();
    const atlas = hosts.find((h) => h.id === "atlas");
    expect([atlas?.agentReport?.projectDirOwner.name, atlas?.agentReport?.fileOwnerSource, atlas?.agentReport?.note]).toEqual(["nobody:users", "manual", "unraid"]);
  });
});

describe("hub address", () => {
  const g = (patch: Partial<GeneralSettings>): GeneralSettings => ({ hubName: "Docklet", hubAddress: "192.0.2.10:8080", domainUrl: "", linkBase: "auto", timeZone: "UTC", ...patch });

  it("accepts internal IPs, host names and URLs", () => {
    expect(["192.0.2.10", "192.0.2.10:8080", "docklet.lan", "http://docklet.lan:8080", "https://hub.example.com"].map(hubAddress)).toEqual([undefined, undefined, undefined, undefined, undefined]);
    expect(["", "two words", "192.0.2.10:99999", "ftp://x"].map(hubAddress)).toEqual(["required", "hubAddress", "hubAddress", "hubAddress"]);
  });

  it("lets agents use the internal address and links prefer an optional domain", () => {
    expect(agentHubUrl(g({ domainUrl: "https://docklet.example.com" }))).toBe("http://192.0.2.10:8080");
    expect(notificationBaseUrl(g({}))).toEqual({ url: "http://192.0.2.10:8080", source: "internal" });
    expect(notificationBaseUrl(g({ domainUrl: "https://docklet.example.com/" }))).toEqual({ url: "https://docklet.example.com", source: "domain" });
    expect(notificationBaseUrl(g({ domainUrl: "https://docklet.example.com", linkBase: "internal" })).source).toBe("internal");
  });
});

describe("deterministic sorting", () => {
  const rows = [
    { id: "3", name: "wiki", host: "Cedar" },
    { id: "1", name: "media", host: "Atlas" },
    { id: "2", name: "Home", host: "Atlas" },
    { id: "4", name: "cloud", host: "Boreal" },
    { id: "5", name: "home", host: "Boreal" },
  ];
  const sort = (key: "host" | "name", dir: "asc" | "desc") =>
    sortRows(rows, { key, dir }, (r, k) => (k === "host" ? r.host : r.name), (r) => ({ id: r.id, name: r.name, hostName: r.host })).map((r) => `${r.host}/${r.name}`);

  it("defaults to host then name and keeps names ascending when the host order flips", () => {
    expect(sort("host", "asc")).toEqual(["Atlas/Home", "Atlas/media", "Boreal/cloud", "Boreal/home", "Cedar/wiki"]);
    expect(sort("host", "desc")).toEqual(["Cedar/wiki", "Boreal/cloud", "Boreal/home", "Atlas/Home", "Atlas/media"]);
  });

  it("breaks name ties by host, case-insensitively and stable across runs", () => {
    expect(sort("name", "asc")).toEqual(["Boreal/cloud", "Atlas/Home", "Boreal/home", "Atlas/media", "Cedar/wiki"]);
    expect(sort("name", "desc")).toEqual(["Cedar/wiki", "Atlas/media", "Atlas/Home", "Boreal/home", "Boreal/cloud"]);
  });

  it("orders hosts by name, not by fixture order", () => {
    expect(sortHosts([{ id: "d", name: "Delta" }, { id: "c", name: "Cedar" }, { id: "a", name: "atlas" }, { id: "x2", name: "Host 10" }, { id: "x1", name: "Host 2" }]).map((h) => h.name)).toEqual(["atlas", "Cedar", "Delta", "Host 2", "Host 10"]);
  });
});
