import { createHash } from "node:crypto";
import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { fileURLToPath } from "node:url";

const root = resolve(fileURLToPath(new URL("..", import.meta.url)));
const specPath = resolve(root, "api/openapi.json");
const typesPath = resolve(root, "web/src/api/generated/schema.ts");

function command(program, args) {
  const result = spawnSync(program, args, {
    cwd: root,
    env: { ...process.env, GOFLAGS: process.env.GOFLAGS || "-p=4" },
    encoding: "utf8",
    maxBuffer: 8 * 1024 * 1024,
  });
  if (result.status !== 0) throw new Error(`${program} failed while generating the API contract`);
}

export function differences(committedSpec, committedTypes, exportedSpec, exportedTypes) {
  return [
    ...(!committedSpec.equals(exportedSpec) ? ["api/openapi.json"] : []),
    ...(!committedTypes.equals(exportedTypes) ? ["web/src/api/generated/schema.ts"] : []),
  ];
}

export function assertNoDrift(committedSpec, committedTypes, exportedSpec, exportedTypes) {
  const changed = differences(committedSpec, committedTypes, exportedSpec, exportedTypes);
  if (changed.length) throw new Error(`API contract drift: ${changed.join(", ")}`);
}

function generatedTypes(spec, workdir) {
  const input = resolve(workdir, "openapi.json");
  const output = resolve(workdir, "schema.ts");
  writeFileSync(input, spec);
  command("pnpm", ["--dir", "web", "exec", "openapi-typescript", input, "--output", output]);
  const revision = createHash("sha256").update(spec).digest("hex");
  const provenance = [
    "// Source: api/openapi.json",
    `// Revision: ${revision}`,
    "// Retrieved: 2026-10-14",
    "// License: BSD-3-Clause",
    "// Deviations: Generated TypeScript declarations from the local Huma export.",
    "",
  ].join("\n");
  return Buffer.from(provenance + readFileSync(output, "utf8"));
}

export function run(mode) {
  if (mode !== "generate" && mode !== "check") throw new Error("expected generate or check");
  const scratch = resolve(root, ".scratch");
  mkdirSync(scratch, { recursive: true });
  const workdir = mkdtempSync(resolve(scratch, "openapi-"));
  try {
    // The Go CLI writes directly to a file; it does not need a server or database.
    const exportPath = resolve(workdir, "export.json");
    command("go", ["run", "./backend/cmd", "openapi", "--format", "json", "--output", exportPath]);
    const spec = readFileSync(exportPath);
    const types = generatedTypes(spec, workdir);
    if (mode === "generate") {
      mkdirSync(resolve(typesPath, ".."), { recursive: true });
      writeFileSync(specPath, spec);
      writeFileSync(typesPath, types);
      const provenancePath = resolve(root, "scripts/provenance.json");
      const provenance = readFileSync(provenancePath, "utf8");
      const digest = createHash("sha256").update(spec).digest("hex");
      const lines = provenance.split("\n");
      const index = lines.findIndex((line) => line.includes('"api/openapi.json":'));
      if (index < 0 || !/"sha256": "[a-f0-9]{64}"/.test(lines[index]))
        throw new Error("missing api/openapi.json provenance record");
      lines[index] = lines[index].replace(/("sha256": ")[a-f0-9]{64}"/, `$1${digest}"`);
      writeFileSync(provenancePath, lines.join("\n"));
      console.log("Generated api/openapi.json and web/src/api/generated/schema.ts");
      return;
    }
    assertNoDrift(readFileSync(specPath), readFileSync(typesPath), spec, types);
    console.log("API contract matches Huma export and generated TypeScript");
  } finally {
    rmSync(workdir, { recursive: true, force: true });
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  try {
    run(process.argv[2]);
  } catch (error) {
    console.error(error.message);
    process.exitCode = 1;
  }
}
