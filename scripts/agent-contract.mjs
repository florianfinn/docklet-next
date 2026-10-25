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
  const update = document.paths["/environments/{id}/updater/projects/{projectId}/update"]?.post;
  if (update) {
    const schema = (suffix) => {
      const name = [suffix, `Updater${suffix}`, `Activity${suffix}`].find((name) => schemas[name]);
      if (!name) throw new Error(`missing lifecycle schema ${suffix}`);
      return [name, schemas[name]];
    };
    const envelope = schema("RuntimeExecutionEnvelope")[1];
    const operations = envelope.properties.operation.enum;
    if (!operations?.includes("update-project") || !operations.includes("restore-project"))
      throw new Error("missing lifecycle execution producer");
    manifest.lifecycleOperations = operations;
    const schemaNames = ["LifecycleRequest", "LifecycleIntent", "ImageSelection", "LifecycleResult", "RestorePoint", "PointImage", "BaselineStatus", "RuntimeOptions", "RuntimeTargetRequest", "RuntimeTargetMetadata"];
    const source = readFileSync(new URL("../backend/pkg/libarcane/edge/commands.go", import.meta.url), "utf8");
    const edgeRoutes = [...source.matchAll(/Method:\s*http.Method(\w+),\s*PathPattern:\s*"([^"]+)",\s*CommandName:\s*"([^"]+)"/g)]
      .filter((match) => /^(updater\.restore\.|updater\.project\.)/.test(match[3]))
      .map((match) => ({ method: match[1].toUpperCase(), path: match[2], command: match[3] }));
    manifest.imageLifecycle = {
      targetKinds: ["project"],
      routes: Object.entries(document.paths).flatMap(([path, item]) => Object.entries(item)
        .filter(([, op]) => ["update-project-images", "restore-update", "get-update-baseline", "list-restore-points", "delete-restore-point", "list-host-restore-points", "delete-host-restore-point"].includes(op?.operationId))
        .map(([method, op]) => ({ method: method.toUpperCase(), path, operationId: op.operationId, sha256: createHash("sha256").update(JSON.stringify(op)).digest("hex") }))),
      schemas: Object.fromEntries(schemaNames.map((name) => {
        const [key, value] = schema(name);
        return [key, { sha256: createHash("sha256").update(JSON.stringify(value)).digest("hex") }];
      })),
      edgeRoutes,
      retiredRoutes: Object.entries(document.paths).flatMap(([path, item]) => Object.entries(item)
        .filter(([, op]) => ["update-project-services", "update-container", "run-updater"].includes(op?.operationId))
        .map(([method, op]) => ({ method: method.toUpperCase(), path, operationId: op.operationId, sha256: createHash("sha256").update(JSON.stringify(op)).digest("hex") }))),
      runtimePreflightSha256: createHash("sha256").update(JSON.stringify(document.paths["/environments/0/runtime-target"].post)).digest("hex"),
      retainedPoints: 5,
      volumeDataIncluded: false,
      normalComposePullsTakeLease: false,
      retiredUpdateCode: "update.path-retired",
      observedIdentity: "target identity, container IDs/names, image IDs, service presence and running state",
      resultSettlement: "atomic with terminal receipt; failed persistence never replays Docker",
    };
    manifest.imageSafety.inactiveCapabilities = manifest.imageSafety.inactiveCapabilities.filter((name) => !["stack-update", "restore"].includes(name));
    manifest.imageSafety.capabilities = [...new Set([...manifest.imageSafety.capabilities, "stack-update", "restore"])];
  }
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
