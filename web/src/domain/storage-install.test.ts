import { describe, expect, it } from "vitest";

import { createTar } from "@/shared/lib/tar";

import { buildInstallArtifacts, type AgentInstallPlan } from "./agent-install";
import { validateFileName } from "./files";
import { isInside } from "./validation";
import { zonedParts } from "./schedule";
import { collectMountUsage, mountRef, nextBackupRun } from "./volumes";

const plan = (patch: Partial<AgentInstallPlan> = {}): AgentInstallPlan => ({
  hostName: "Werkstatt",
  connection: "direct",
  address: "werkstatt.example.com",
  port: 3552,
  hubUrl: "http://192.0.2.10:8080",
  projectDir: "/home/docker",
  installDir: "/home/docker/docklet-agent",
  token: "DEMO-NUR-ANSICHT-TEST",
  timeZone: "Europe/Berlin",
  ...patch,
});

describe("agent installation artifacts", () => {
  it("opens the agent port only for direct connections", () => {
    const direct = buildInstallArtifacts(plan());
    const edge = buildInstallArtifacts(plan({ connection: "edge" }));
    expect(direct.compose.includes('"3552:3552"')).toBe(true);
    expect(direct.env.includes("DOCKLET_HUB_URL")).toBe(false);
    expect(edge.compose.includes("ports:")).toBe(false);
    expect(edge.env.includes("DOCKLET_HUB_URL=http://192.0.2.10:8080")).toBe(true);
  });

  it("installs the agent into its own folder inside the project directory and labels it as a system component", () => {
    const a = buildInstallArtifacts(plan());
    expect(a.compose.includes("- /home/docker:/home/docker")).toBe(true);
    expect(a.compose.includes('io.docklet.system: "agent"')).toBe(true);
    expect(a.scriptCommand.startsWith("cat > /tmp/install-docklet-agent.sh")).toBe(true);
    expect(a.script.includes(`INSTALL_DIR='/home/docker/docklet-agent'`) && a.script.includes('cd "$INSTALL_DIR"')).toBe(true);
    expect(a.composeCommand.includes('mkdir -p "$INSTALL_DIR/data" && cd "$INSTALL_DIR"')).toBe(true);
    // The installer refuses the project root itself and says what it checks locally.
    expect(a.script.includes('if [ "$INSTALL_DIR" = "$PROJECT_DIR" ]; then') && a.script.includes("# Local checks on this host")).toBe(true);
    expect(a.readme.includes("io.docklet.system=agent")).toBe(true);
  });

  it("detects file owner and socket group separately and never changes ownership", () => {
    const auto = buildInstallArtifacts(plan());
    expect(auto.script.includes('stat -c %u "$PROJECT_DIR"') && auto.script.includes("stat -c %g /var/run/docker.sock")).toBe(true);
    expect(/^\s*(sudo\s+)?(chown|chmod\s+-R)\b/m.test(auto.script + auto.composeCommand)).toBe(false);
    const manual = buildInstallArtifacts(plan({ fileOwner: { uid: 1000, gid: 1001 }, socketGid: 998 }));
    expect(manual.env.includes("PUID=1000") && manual.env.includes("PGID=1001") && manual.env.includes("DOCKER_SOCKET_GID=998")).toBe(true);
    // Manual IDs are written as given; stat only reports the owner for the log.
    expect(manual.script.includes('PUID="${PUID:-$(stat') || manual.script.includes("stat -c %g /var/run/docker.sock")).toBe(false);
  });

  it("marks the token as a placeholder", () => {
    expect(buildInstallArtifacts(plan()).env.includes("DOCKLET_AGENT_TOKEN=DEMO-NUR-ANSICHT-TEST")).toBe(true);
  });
});

describe("tar package", () => {
  it("writes valid ustar headers, padded blocks and the end marker", () => {
    const tar = createTar([{ name: "docklet-agent/compose.yaml", content: "name: x\n" }], 0);
    expect(tar.length).toBe(512 + 512 + 1024);
    const text = (from: number, len: number) => new TextDecoder().decode(tar.subarray(from, from + len)).replace(/\0.*$/s, "");
    expect(text(0, 100)).toBe("docklet-agent/compose.yaml");
    expect(parseInt(text(124, 12), 8)).toBe(8);
    expect(text(257, 6)).toBe("ustar");
    let sum = 0;
    for (let i = 0; i < 512; i++) sum += i >= 148 && i < 156 ? 32 : (tar[i] ?? 0);
    expect(parseInt(text(148, 8), 8)).toBe(sum);
    expect(text(512, 8)).toBe("name: x\n");
  });
});

describe("storage model", () => {
  const containers = [
    { id: "c1", name: "media-jellyfin-1", hostId: "atlas", stackId: "media", serviceName: "jellyfin", state: "running", mounts: ["media_config:/config", "/mnt/user/media:/media:ro"] },
    { id: "c2", name: "other", hostId: "atlas", stackId: "monitoring", serviceName: "prometheus", state: "stopped", mounts: ["./prometheus.yml:/etc/prometheus/prometheus.yml:ro", "/mnt/user/media:/data"] },
    { id: "c3", name: "anon", hostId: "atlas", state: "running", mounts: ["/data"] },
  ];

  it("groups mounts into volumes and bind mounts with their consumers", () => {
    const usage = collectMountUsage(containers, () => "/srv/stacks");
    const vol = usage.find((u) => u.kind === "volume");
    expect(vol?.name).toBe("media_media_config");
    expect(vol?.composeName).toBe("media_config");
    const media = usage.find((u) => u.name === "/mnt/user/media");
    expect(media?.consumers.map((c) => [c.containerId, c.readOnly, c.running])).toEqual([
      ["c1", true, true],
      ["c2", false, false],
    ]);
    const file = usage.find((u) => u.fileBind);
    expect(file?.name).toBe("/srv/stacks/monitoring/prometheus.yml");
    expect(usage.length).toBe(3);
  });

  it("links a container mount to the same storage entry", () => {
    const ref = mountRef(containers[0]!, "/mnt/user/media:/media:ro", "/srv/stacks");
    expect(ref?.id).toBe(collectMountUsage(containers, () => "/srv/stacks").find((u) => u.name === "/mnt/user/media")?.id);
  });

  it("computes the next scheduled backup in the hub time zone", () => {
    const tz = "Europe/Berlin";
    const from = new Date("2026-10-10T02:00:00Z"); // Saturday 04:00 in Berlin (CEST)
    const at = (d: Date | undefined) => (d ? `${zonedParts(d, tz).date} ${zonedParts(d, tz).time}` : undefined);
    expect(nextBackupRun({ mode: "manual" }, from, tz)).toBe(undefined);
    expect(at(nextBackupRun({ mode: "daily", time: "03:00" }, from, tz))).toBe("2026-10-11 03:00");
    expect(at(nextBackupRun({ mode: "daily", time: "05:30" }, from, tz))).toBe("2026-10-10 05:30");
    expect(at(nextBackupRun({ mode: "weekly", day: "sun", time: "04:00" }, from, tz))).toBe("2026-10-11 04:00");
  });

  it("validates file names and directory nesting", () => {
    expect(validateFileName("a/b", [])).toBe("invalid");
    expect(validateFileName("notes.txt", ["notes.txt"])).toBe("exists");
    expect(validateFileName("notes.txt", ["notes.txt"], "notes.txt")).toBe(undefined);
    expect(isInside("/home/docker/agent", "/home/docker") && !isInside("/home/dockerx", "/home/docker")).toBe(true);
  });
});
