import assert from "node:assert/strict";
import { fileURLToPath } from "node:url";
import { build, type Plugin } from "vite";

const root = fileURLToPath(new URL("..", import.meta.url));
const configFile = fileURLToPath(new URL("../vite.config.ts", import.meta.url));

async function inspect(injected?: string, mode = "production", target = "/src/main.tsx", extraCode = "") {
  const injection: Plugin = {
    name: "production-boundary-red-proof",
    enforce: "pre",
    transform(code, id) {
      if (id.endsWith(target) && (injected || extraCode)) return `${injected ? `import ${JSON.stringify(injected)};\n` : ""}${code}\n${extraCode}`;
    },
  };
  return build({ root, configFile, mode, logLevel: "silent", plugins: [injection], build: { write: false } });
}

const result = await inspect();
assert(!Array.isArray(result) && "output" in result);
const evidence = result.output.find((entry) => entry.type === "asset" && entry.fileName === "production-modules.json");
assert(evidence?.type === "asset", "Production graph evidence missing");
const graph = JSON.parse(String(evidence.source)) as { graph: string[]; shipped: string[] };
assert(graph.shipped.includes("src/data/disconnected-adapter.ts"), "Production adapter missing");
assert(graph.shipped.includes("src/shared/preferences/FavoritesSection.tsx"), "Product design-code controls missing");
assert(!graph.graph.some((id) => /data\/demo\/|features\/design-lab\/|runtime\.dev\./.test(id)), "Forbidden production module");
console.log(`Production boundary passed: ${graph.graph.length} source modules checked, ${graph.shipped.length} shipped; disconnected adapter and preference controls included.`);

if (process.argv.includes("--prove")) {
  for (const [injected, mode] of [
    ["@/data/demo/simulation", "production"],
    ["@/features/design-lab/DesignLabPanel", "production"],
    ["@/data/demo/simulation", "development"],
  ]) {
    await assert.rejects(inspect(injected, mode), /Production boundary: forbidden module/);
    console.log(`Red proof passed: injected ${injected} rejected in ${mode} build.`);
  }
  await assert.rejects(inspect("@/data/demo/demo-text", "production", "/src/shared/hooks/use-stack-commands.ts"), /Production boundary: forbidden module/);
  console.log("Red proof passed: shared hook import of demo text rejected in production build.");
  await assert.rejects(inspect("@/test/layout"), /Production boundary: forbidden module/);
  console.log("Red proof passed: test helper rejected in production build.");
  for (const module of [
    "@/features/hosts/InstallSheet.dev",
    "@/features/setup/SetupPage",
    "@/features/hosts/onboarding/OnboardingPage",
    "@/app/preview/PreviewPage",
  ]) {
    await assert.rejects(inspect(module), /Production boundary: forbidden module/);
    console.log(`Red proof passed: injected ${module} rejected in production build.`);
  }
  await assert.rejects(inspect(undefined, "production", "/src/main.tsx", 'document.title = "Design-Labor";'), /Production boundary: forbidden text/);
  console.log("Red proof passed: lab text rejected in production assets.");
}
