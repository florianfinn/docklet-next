// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original installer tests for the Edge tailnet route.

// @vitest-environment node
import { describe, expect, it } from "vitest";

import { buildInstallArtifacts, installPackageFiles, tailnetHostname, type AgentInstallPlan } from "./agent-install";

const plan: AgentInstallPlan = {
  hostName: "Example host", connection: "edge", address: "agent.example.com", port: 3552,
  hubUrl: "http://192.0.2.1:3552", projectDir: "/srv/projects", installDir: "/srv/projects/docklet-agent",
  token: "test-only", timeZone: "UTC", image: "registry.example.com/docklet/agent:0.1.0",
  tailnet: { authKey: "tskey-auth-kExample-HOSTKEY", hostname: "docklet-example-host" },
};

describe("tailnet route installer", () => {
  it("renders the TAILSCALE keys only into the private .env", () => {
    const a = buildInstallArtifacts(plan);
    expect(a.env).toContain("TAILSCALE_AUTH_KEY='tskey-auth-kExample-HOSTKEY'\n");
    expect(a.env).toContain("TAILSCALE_HOSTNAME='docklet-example-host'\n");
    expect(a.env).toContain("TAILSCALE_STATE_DIR=/app/data/tailscale\n");
    expect(a.env).toContain("MANAGER_API_URL='http://192.0.2.1:3552'\n");
    expect(a.env).not.toContain("TAILSCALE_CONTROL_URL");
    expect(a.compose).not.toContain("HOSTKEY");
    expect(a.readme).not.toContain("HOSTKEY");
    expect(a.compose).toContain("./data:/app/data");
    const files = installPackageFiles(a);
    expect(files.find((f) => f.name === ".env")?.mode).toBe(0o600);
    expect(a.script).toContain('chmod 600 "$WORK_DIR/.env"');
  });

  it("adds the control URL for self-hosted control servers", () => {
    const a = buildInstallArtifacts({ ...plan, tailnet: { ...plan.tailnet!, controlUrl: "https://control.example.com" } });
    expect(a.env).toContain("TAILSCALE_CONTROL_URL='https://control.example.com'\n");
  });

  it("leaves non-tailnet installers unchanged", () => {
    const a = buildInstallArtifacts({ ...plan, tailnet: undefined, hubUrl: "http://hub.example.com" });
    expect(a.env).not.toContain("TAILSCALE_");
  });

  it.each([
    ["an OAuth client secret", { ...plan, tailnet: { ...plan.tailnet!, authKey: "tskey-client-kExample-SECRET" } }, "agent-install.invalid-tailnet-key"],
    ["a key with a newline", { ...plan, tailnet: { ...plan.tailnet!, authKey: "tskey-auth-a\nB=1" } }, "agent-install.invalid-tailnet-key"],
    ["a Direct host", { ...plan, connection: "direct" as const }, "agent-install.invalid-tailnet"],
    ["an https hub URL", { ...plan, hubUrl: "https://192.0.2.1:3552" }, "agent-install.invalid-tailnet"],
    ["an invalid node name", { ...plan, tailnet: { ...plan.tailnet!, hostname: "Docklet Host" } }, "agent-install.invalid-tailnet"],
    ["a control URL with credentials", { ...plan, tailnet: { ...plan.tailnet!, controlUrl: "https://user:pass@control.example.com" } }, "agent-install.invalid-tailnet"],
  ])("refuses %s", (_, bad, code) => {
    expect(() => buildInstallArtifacts(bad)).toThrow(code);
  });

  it("derives a DNS-label node name from the host name", () => {
    expect(tailnetHostname("Example host")).toBe("docklet-example-host");
    expect(tailnetHostname("Bücher_Server 2")).toBe("docklet-bucher-server-2");
    expect(tailnetHostname("!!!")).toBe("docklet-agent");
    expect(tailnetHostname("x".repeat(80)).length).toBeLessThanOrEqual(63);
  });
});
