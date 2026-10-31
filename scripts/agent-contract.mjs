import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";

const fixtureURL = new URL("../types/contract/testdata/agent-hub-v11.json", import.meta.url);
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
      const name = [suffix, `Updater${suffix}`, `Activity${suffix}`, `Jobschedule${suffix}`, `Scheduler${suffix}`, `Version${suffix}`].find((name) => schemas[name]);
      if (!name) throw new Error(`missing lifecycle schema ${suffix}`);
      return [name, schemas[name]];
    };
    const envelope = schema("RuntimeExecutionEnvelope")[1];
    const operations = envelope.properties.operation.enum;
    if (!operations?.includes("update-project") || !operations.includes("restore-project"))
      throw new Error("missing lifecycle execution producer");
    manifest.lifecycleOperations = operations;
    const schemaNames = ["LifecycleRequest", "LifecycleIntent", "PolicyBinding", "ImageSelection", "LifecycleResult", "RestorePoint", "PointImage", "BaselineStatus", "RuntimeOptions", "RuntimeTargetRequest", "RuntimeTargetMetadata"];
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
      retentionProtections: schema("RestorePoint")[1].properties.protectionReason.enum.filter((value) => value !== null),
      holdStates: schema("RestorePoint")[1].properties.holdState.enum,
      pointRequiredFields: schema("RestorePoint")[1].required,
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
    if (document.paths["/environments/{id}/updater/schedules"]) {
      manifest.imagePolicies = {
        targetKinds: ["project"], defaultMode: "off", modes: ["off", "notify", "apply"],
        owner: "hub", jobs: ["image-polling", "auto-update"], timeZone: "hub IANA zone",
        routes: Object.entries(document.paths).flatMap(([path, item]) => Object.entries(item)
          .filter(([, op]) => ["get-auto-update-policy", "set-auto-update-policy", "get-update-schedules", "set-update-schedules", "get-system-image-hints"].includes(op?.operationId))
          .map(([method, op]) => ({ method: method.toUpperCase(), path, operationId: op.operationId, sha256: createHash("sha256").update(JSON.stringify(op)).digest("hex") }))),
        schemas: Object.fromEntries(["ProjectUpdateInfo", "AutoUpdatePolicy", "UpdatePolicyInput", "UpdateCandidate", "UpdateScheduleConfig", "UpdateScheduleInput", "JobscheduleConfig", "JobscheduleUpdate", "JobStatus", "Outcome", "TargetOutcome", "SystemImageHint", "SystemImageHints", "VersionInfo"].map((name) => {
          const [key, value] = schema(name);
          return [key, { sha256: createHash("sha256").update(JSON.stringify(value)).digest("hex") }];
        })),
        admission: "current actor/key, registration, baseline and revision; freeze revision in runtime receipt transaction",
        execution: "existing update-project runtime; serial children per host; retry/restart queries original receipts only",
        agentLocalCode: "update.agent-local-job-disabled", legacyScheduleWriteCode: "update.schedule-hub-owned",
        legacyScheduleFields: "autoUpdateInterval and pollingInterval omitted from reads/schema; any write including null or mixed fields refuses before forwarding",
        notificationIdentity: ["host", "registration", "stack identity", "repository", "platform", "normalized version/tag", "immutable digest"],
        notificationCommit: "candidate dedup row and linked durable notification entry in one hub transaction",
        notificationRecovery: "reuse entry across checks, restart and policy toggles; failed/unknown delivery never creates another entry",
      };
      manifest.imageSafety.inactiveCapabilities = manifest.imageSafety.inactiveCapabilities.filter((name) => !["policies", "auto-apply", "notifications", "hub-schedules", "hub-image-schedules"].includes(name));
      manifest.imageSafety.capabilities = [...new Set([...manifest.imageSafety.capabilities.filter((name) => !["stack-policies", "hub-schedules"].includes(name)), "policies", "auto-apply", "hub-image-schedules", "candidate-notifications", "system-image-hints"])];
    }
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
  manifest.version = 11;
  manifest.minimumAgentContractVersion = 11;
  const vulnerabilityRoutes = Object.entries(document.paths).flatMap(([path, item]) => Object.entries(item)
    .filter(([, op]) => op?.tags?.includes("Vulnerabilities"))
    .map(([method, op]) => ({ method: method.toUpperCase(), path, operationId: op.operationId, sha256: createHash("sha256").update(JSON.stringify(op)).digest("hex") })));
  const vulnerabilitySchemas = {};
  function collectVulnerability(value) {
    if (!value || typeof value !== "object") return;
    if (value.$ref) {
      const name = value.$ref.split("/").at(-1);
      if (!vulnerabilitySchemas[name]) {
        vulnerabilitySchemas[name] = { sha256: createHash("sha256").update(JSON.stringify(schemas[name])).digest("hex") };
        collectVulnerability(schemas[name]);
      }
    }
    for (const child of Object.values(value)) {
      if (Array.isArray(child)) child.forEach(collectVulnerability); else collectVulnerability(child);
    }
  }
  for (const route of vulnerabilityRoutes) collectVulnerability(document.paths[route.path][route.method.toLowerCase()]);
  const edgeSource = readFileSync(new URL("../types/vulnerability/routes.go", import.meta.url), "utf8");
  manifest.vulnerability = {
    routes: vulnerabilityRoutes,
    schemas: vulnerabilitySchemas,
    edgeRoutes: [...edgeSource.matchAll(/Method:\s*"(\w+)",\s*Path:\s*"([^"\n]+)",\s*CommandName:\s*"([^"\n]+)"/g)]
      .map(([, method, path, command]) => ({method: method.toUpperCase(), path: `/api${path}`, command})),
    responseLimit: 32 << 20, reportLimit: 64 << 20,
    admission: "one host operation; busy includes running activity ID",
    baseline: "last completed image findings survive failed attempts",
  };
  const volumeRoutes = Object.entries(document.paths).flatMap(([path, item]) => Object.entries(item)
    .filter(([, op]) => op?.operationId && (/^\/backup-targets(?:\/|$)/.test(path) || /^\/environments\/\{id\}\/volumes\/(?:backups(?:\/|$)|\{volumeName\}\/(?:backups|backup-policy))/.test(path)))
    .map(([method, op]) => ({ method: method.toUpperCase(), path, operationId: op.operationId, sha256: createHash("sha256").update(JSON.stringify(op)).digest("hex") })));
  const volumeSchemas = {};
  function collectVolume(value) {
    if (!value || typeof value !== "object") return;
    if (value.$ref) {
      const name = value.$ref.split("/").at(-1);
      if (!volumeSchemas[name]) {
        if (!schemas[name]) throw new Error(`missing volume schema ${name}`);
        volumeSchemas[name] = {sha256:createHash("sha256").update(JSON.stringify(schemas[name])).digest("hex")};
        collectVolume(schemas[name]);
      }
    }
    for (const child of Object.values(value)) {
      if (Array.isArray(child)) child.forEach(collectVolume); else collectVolume(child);
    }
  }
  for (const route of volumeRoutes) collectVolume(document.paths[route.path][route.method.toLowerCase()]);
  for (const name of ["BackupTargetSet", "BackupTargetSetReceipt", "BackupProblem"]) collectVolume({$ref:`#/components/schemas/${name}`});
  const commands = readFileSync(new URL("../backend/pkg/libarcane/edge/commands.go", import.meta.url), "utf8");
  manifest.volumeBackups = {
    routes:volumeRoutes, schemas:volumeSchemas,
    edgeRoutes:[...commands.matchAll(/Method:\s*http.Method(\w+),\s*PathPattern:\s*"([^"]+)",\s*CommandName:\s*"([^"]+)"/g)]
      .filter((match) => /^volume\.backup/.test(match[3]))
      .map(([, method,path,command])=>({method:method.toUpperCase(),path,command})),
    errorCodes:schemas.BackupProblem.properties.code.enum,
    uploadReasons:schemas.BackupProblem.properties.reason.enum.filter((value)=>value!==null),
    responseLimit:8<<20, edgeDownloadLimit:512<<20, uploadLimit:4*2**30, expandedUploadLimit:16*2**30, browseEntries:5000,
    restoreStatus:202, policyLimit:1, targetAuthority:"UUID generation, preserved restored version counter", localRecovery:"verify before attach; never re-key",
    retiredWorkspaceOperation:"restore_file", retiredRoutes:["/backups/volumes/config","/backups/volumes/options","/backups/volumes/run"],
  };
  manifest.autoHeal = {
    routes: Object.entries(document.paths).flatMap(([path,item])=>Object.entries(item).filter(([,op])=>["get-auto-heal-settings","update-auto-heal-settings"].includes(op?.operationId)).map(([method,op])=>({method:method.toUpperCase(),path,operationId:op.operationId,sha256:createHash("sha256").update(JSON.stringify(op)).digest("hex")}))),
    settings: schemas.AutohealSettings, update: schemas.AutohealUpdate,
    edgeCommands:["auto_heal.settings.get","auto_heal.settings.update"],
  };
  manifest.notificationDispatch = { request:schemas.NotificationDispatchRequest, autoHeal:schemas.NotificationDispatchAutoHeal, backupFailed:schemas.NotificationDispatchBackupFailed, origin:"captured credential and Edge session; transaction rechecks binding", legacy:"direct fan-out, no eventId", hostEvents:"best effort; no agent outbox" };
  const names = ["OriginAssignment", "RecreateRequest", "RecreateResult", "ContainerProblem", "ShellQuery", "ShellClose", "ShellResize", "ShellExit", "AgentRestartResponse", "AgentRestartRun", "HostProblem", "HealthResponse", "RuntimeOptions", "RuntimeSnapshot"];
  manifest.containerHostOperations = {
    routes: Object.entries(document.paths).flatMap(([path,item])=>Object.entries(item)
      .filter(([,op])=>["assign-container-origin","reset-container-origin","recreate-container","redeploy-container","stop-project","restartEnvironmentAgent","getEnvironmentAgentRestart"].includes(op?.operationId))
      .map(([method,op])=>({method:method.toUpperCase(),path,operationId:op.operationId,sha256:createHash("sha256").update(JSON.stringify(op)).digest("hex")}))),
    schemas: Object.fromEntries(names.map(name => {
      const key = [name, `Container${name}`, `Environment${name}`, `System${name}`, `Activity${name}`].find(key => schemas[key]);
      if (!schemas[key]) throw new Error(`missing M14 schema ${name}`);
      return [key, schemas[key]];
    })),
    runtimeOperations: schemas.ActivityRuntimeExecutionEnvelope.properties.operation.enum,
    origin: schemas.ContainerSummary.properties.origin,
    originSource: schemas.ContainerSummary.properties.originSource,
    stackServiceState: schemas.ProjectRuntimeService.properties.status, stackHint: schemas.ProjectDetails.properties.definitionHint,
    hostFields: {name: schemas.EnvironmentUpdate.properties.name, color: schemas.EnvironmentUpdate.properties.color, read: schemas.EnvironmentEnvironment.properties.color},
    settings: schemas.SettingsUpdate.properties.stackStopMode,
    stackStopMode: {values:["down","stop"], default:"down", environment:"0"},
    shell: {frameProtocolVersion:1, binary:"stdin/stdout", text:"resize/close/exit", closeCodes:[1000,1008], wsStart:"allowlisted", closeReasonTransport:true},
    restart: {method:"POST", path:"/api/system/agent/restart", command:"system.agent.restart", route:"agent-only 202 after inspected self resolution; hub one-shot persisted observer", hubIntake:"POST /environments/{id}/agent/restart", hubRead:"GET /environments/{id}/agent/restarts/{runId}", recovery:"reserved interrupted; sent/awaiting observe without resend", acknowledgement:"durable acknowledged_at; response instance is evidence only"},
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
    if (!current.equals(fixture) || source !== updated) throw new Error("Agent contract fixture or fingerprint drift");
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
  if (Object.values(value).every((item) => item === null || typeof item !== "object")) return JSON.stringify(value);
  const entries = Object.entries(value);
  if (!entries.length) return "{}";
  return `{\n${entries.map(([key, item]) => `${child}${JSON.stringify(key)}: ${formatFixture(item, depth + 1)}`).join(",\n")}\n${indent}}`;
}
