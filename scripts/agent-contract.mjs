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
  manifest.imageSafety.errorCodes = schemas.ImageProblem.properties.code.enum;
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
      retentionProtections: ["latest-restore-target", "latest-healthy-baseline", "active-or-unknown-run"],
      failedAttemptPoints: 1,
      failedAttemptScope: "partial-source from failed attempts; healthy baseline and successful pre-restore points exempt",
      volumeDataIncluded: false,
      normalComposePullsTakeLease: false,
      retiredUpdateCode: "update.path-retired",
      observedIdentity: "target identity and modeled/extra container/image IDs and labels; running state is recorded only",
      configurationProof: "saved Compose/env bytes, rendered model, config-hash labels and image bindings; no runtime state",
      runtimeEvidence: "all containers are captured; only configured non-one-off service and replica images require inspection and pins",
      resultSettlement: "atomic with terminal receipt; failed persistence never replays Docker",
    };
    manifest.imageSafety.inactiveCapabilities = manifest.imageSafety.inactiveCapabilities.filter((name) => !["stack-update", "restore"].includes(name));
    manifest.imageSafety.capabilities = [...new Set([...manifest.imageSafety.capabilities, "stack-update", "restore"])];
  }
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
  return Buffer.from(`${formatFixture(manifest)}\n`);
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

// Scalar arrays stay on one line so the unreleased fixture remains reviewable
// within the repository line ceiling without dropping any contract fields.
function formatFixture(value, depth = 0) {
  if (value === null || typeof value !== "object") return JSON.stringify(value);
  const indent = "  ".repeat(depth);
  const child = `${indent}  `;
  if (Array.isArray(value)) {
    if (value.every((item) => item === null || typeof item !== "object")) return JSON.stringify(value);
    return `[\n${value.map((item) => child + formatFixture(item, depth + 1)).join(",\n")}\n${indent}]`;
  }
  const entries = Object.entries(value);
  if (!entries.length) return "{}";
  return `{\n${entries.map(([key, item]) => `${child}${JSON.stringify(key)}: ${formatFixture(item, depth + 1)}`).join(",\n")}\n${indent}}`;
}
