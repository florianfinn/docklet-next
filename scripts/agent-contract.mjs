import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";

const fixtureURL = new URL("../types/contract/testdata/agent-hub-v7.json", import.meta.url);
const agentURL = new URL("../types/contract/agent.go", import.meta.url);

// Preserve the accepted runtime and image manifest; derive the optional HTTP
// discovery extension from the same registered producer as the generated DTOs.
export function agentFixture(spec, fixture) {
  const document = JSON.parse(spec);
  const manifest = JSON.parse(fixture);
  const path = "/environments/{id}/activities";
  const operation = document.paths[path].get;
  const schemas = document.components.schemas;
  const responseSchema = operation.responses["200"].content["application/json"].schema;
  const page = schemas[responseSchema.$ref.split("/").at(-1)];
  const summary = schemas[page.properties.data.items.$ref.split("/").at(-1)];
  const recovery = schemas[summary.properties.recovery.$ref.split("/").at(-1)];
  const include = operation.parameters.find((parameter) => parameter.name === "includeSummary");
  if (!include || !summary || !recovery) throw new Error("missing activity discovery producer");
  manifest.activityDiscovery = {
    route: `GET ${path}`,
    includeParameter: include,
    responseSchema,
    fields: Object.keys(summary.properties).sort(),
    optionalFields: Object.keys(summary.properties).filter((name) => !summary.required.includes(name)).sort(),
    knowledgeDependencies: summary.dependentRequired,
    stepTrace: summary.properties.stepTrace,
    recovery: summary.properties.recovery,
    recoveryProperties: recovery.properties,
  };
  const pull = document.paths["/environments/{id}/images/pull"].post.responses["200"];
  const frame = pull.content["application/x-json-stream"].schema;
  const frameSchema = frame.$ref ? schemas[frame.$ref.split("/").at(-1)] : frame;
  const definitions = {};
  function collect(schema) {
    if (!schema || typeof schema !== "object") return;
    if (schema.$ref) {
      const name = schema.$ref.split("/").at(-1);
      if (!definitions[name]) {
        definitions[name] = schemas[name];
        collect(schemas[name]);
      }
    }
    for (const child of Object.values(schema)) {
      if (Array.isArray(child)) child.forEach(collect);
      else collect(child);
    }
  }
  if (!frameSchema?.oneOf?.length) throw new Error("missing pull stream frame producer");
  collect(frame);
  manifest.imageSafety.pullStream = {
    route: "POST /environments/{id}/images/pull",
    mediaType: "application/x-json-stream",
    frame,
    schemas: definitions,
    registryAuthenticationStatus: 502,
  };
  return Buffer.from(`${JSON.stringify(manifest, null, 2)}\n`);
}

export function updateAgentContract(spec, mode) {
  const current = readFileSync(fixtureURL);
  const fixture = agentFixture(spec, current);
  const digest = createHash("sha256").update(fixture).digest("hex");
  const source = readFileSync(agentURL, "utf8");
  const updated = source.replace(/const AgentHubFingerprint = "[a-f0-9]{64}"/, `const AgentHubFingerprint = "${digest}"`);
  if (mode === "check") {
    if (!current.equals(fixture) || source !== updated) throw new Error("Agent contract v7 fixture or fingerprint drift");
    return;
  }
  writeFileSync(fixtureURL, fixture);
  writeFileSync(agentURL, updated);
}
