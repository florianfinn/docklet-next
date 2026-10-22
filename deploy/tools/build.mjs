// Source: docklet-next/issues/180
// Revision: 50517cb1043719584cdd3ce940f0c8abb1497ab8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original dependency-free buildx wrapper; all version pins come from inputs.env.
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";

export function inputs() {
  return Object.fromEntries(readFileSync(new URL("inputs.env", import.meta.url), "utf8")
    .split("\n").filter((line) => line && !line.startsWith("#")).map((line) => {
      const match = /^([A-Z][A-Z0-9_]*)=([a-zA-Z0-9:.,/@_-]+)$/.exec(line);
      if (!match) throw new Error("invalid tools build input");
      return match.slice(1);
    }));
}
if (process.argv[1] && import.meta.url === new URL(process.argv[1], "file:").href) {
  const args = Object.entries(inputs()).flatMap(([key, value]) => ["--build-arg", `${key}=${value}`]);
  const result = spawnSync("docker", ["buildx", "build", "--file", "deploy/tools/Dockerfile",
    ...args, ...process.argv.slice(2), "."], { stdio: "inherit" });
  process.exit(result.status ?? 1);
}
