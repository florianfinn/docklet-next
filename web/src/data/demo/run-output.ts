import type { DeployStepKey, OutputStream, RunOutputLine } from "@/domain/model";

/**
 * Simulated raw output of Docker and Docker Compose for demo actions. The
 * lines imitate the CLI's shape (Compose writes progress to stderr) but are
 * generated here; nothing calls Docker.
 */
export interface OutputSpec {
  stream: OutputStream;
  text: string;
  level?: RunOutputLine["level"];
}

export interface OutputStep {
  key: DeployStepKey;
  label: string;
  detail?: string;
}

export interface OutputScript {
  /** Lines while the step runs, spread over its duration. */
  lines(step: OutputStep): OutputSpec[];
  /** Diagnosis appended when the step fails. */
  failure(step: OutputStep, message: string): OutputSpec[];
}

export const SIMULATED_NOTE = "Simulierte Ausgabe – kein echter Docker-Aufruf, Werte sind erfunden.";

const out = (text: string): OutputSpec => ({ stream: "stdout", text });
const err = (text: string, level?: OutputSpec["level"]): OutputSpec => ({ stream: "stderr", text, level });
const cmd = (text: string): OutputSpec => ({ stream: "cmd", text });

/** Deterministic hex id from a string, so layers and digests stay stable per image. */
export function fakeHex(seed: string, length = 12): string {
  let h = 2166136261;
  let hex = "";
  for (let round = 0; hex.length < length; round++) {
    for (let i = 0; i < seed.length; i++) h = Math.imul(h ^ seed.charCodeAt(i), 16777619);
    h = Math.imul(h ^ round, 16777619);
    hex += (h >>> 0).toString(16).padStart(8, "0");
  }
  return hex.slice(0, length);
}

/** Docker's 50-column progress bar, e.g. `[=====>      ]`. */
function progressBar(fraction: number): string {
  const filled = Math.max(1, Math.round(fraction * 50));
  return `[${"=".repeat(filled - 1)}>${" ".repeat(50 - filled)}]`;
}

/** Layer progress as Docker emits it: interleaved frames per layer that a terminal rewrites in place. */
function layerLines(service: string, image: string): OutputSpec[] {
  const layers = [0, 1, 2].map((i) => fakeHex(`${image}#${i}`));
  const sizes = [3.4, 28.7, 61.2];
  const frames = [0.12, 0.38, 0.67, 0.91];
  return [
    err(` ${service} Pulling`),
    ...layers.map((l) => err(` ${l} Pulling fs layer`)),
    ...frames.flatMap((f) => layers.map((l, i) => err(` ${l} Downloading ${progressBar(f)}  ${(sizes[i]! * f).toFixed(1)}MB/${sizes[i]}MB`))),
    ...layers.flatMap((l, i) => [
      err(` ${l} Download complete`),
      err(` ${l} Extracting ${progressBar(0.5)}  ${(sizes[i]! * 0.5).toFixed(1)}MB/${sizes[i]}MB`),
      err(` ${l} Extracting ${progressBar(1)}  ${sizes[i]}MB/${sizes[i]}MB`),
      err(` ${l} Pull complete`),
    ]),
    err(` ${service} Pulled`),
  ];
}

export interface ComposeTarget {
  project: string;
  services: { name: string; image: string }[];
  /** Container name pattern of Compose: project-service-1. */
  hostName: string;
}

const containerName = (project: string, service: string) => `${project}-${service}-1`;

/** Compose stack actions: config check, pull, up, stop, start and health inspection. */
export function composeScript(target: ComposeTarget, scope?: string[]): OutputScript {
  const p = target.project;
  const services = scope?.length ? target.services.filter((s) => scope.includes(s.name)) : target.services;
  const names = services.map((s) => containerName(p, s.name));
  return {
    lines(step) {
      switch (step.key) {
        case "validate":
          return [cmd(`docker compose --project-name ${p} config --quiet`), out(`Compose-Datei gültig · ${target.services.length} Services · Host ${target.hostName}`)];
        case "pull":
          return [cmd(`docker compose --project-name ${p} pull ${services.map((s) => s.name).join(" ")}`), ...services.flatMap((s) => layerLines(s.name, s.image))];
        case "create":
        case "build":
          return [cmd(`docker compose --project-name ${p} up --detach --no-start --remove-orphans`), ...names.flatMap((n) => [err(` Container ${n}  Recreate`), err(` Container ${n}  Recreated`)])];
        case "stop":
          return [cmd(`docker compose --project-name ${p} stop`), ...names.flatMap((n) => [err(` Container ${n}  Stopping`), err(` Container ${n}  Stopped`)])];
        case "start":
          return [cmd(`docker compose --project-name ${p} start`), ...names.flatMap((n) => [err(` Container ${n}  Starting`), err(` Container ${n}  Started`)])];
        case "health":
          return [cmd(`docker inspect --format '{{.Name}} {{.State.Status}} {{if .State.Health}}{{.State.Health.Status}}{{end}}' ${names.join(" ")}`), ...names.map((n) => out(`/${n} running healthy`))];
        default:
          return genericLines(step);
      }
    },
    failure(step, message) {
      if (step.key === "pull") return [err(`Error response from daemon: pull failed for ${services[0]?.image ?? p} (simuliert)`, "error"), err(message, "error")];
      if (step.key === "health") return [err(`${names[0] ?? p} unhealthy · Healthcheck exit 1 – Service-Logs prüfen`, "error"), err(message, "error")];
      if (step.key === "validate") return [err(`error during connect: Agent auf ${target.hostName} nicht erreichbar (simuliert)`, "error"), err(message, "error")];
      return [err(`Error response from daemon: ${message}`, "error")];
    },
  };
}

/** Single container actions with the plain Docker CLI. */
export function containerScript(name: string, image: string): OutputScript {
  return {
    lines(step) {
      switch (step.key) {
        case "stop":
          return [cmd(`docker stop ${name}`), out(name)];
        case "start":
          return [cmd(`docker start ${name}`), out(name)];
        case "pull":
          return [cmd(`docker pull ${image}`), ...layerLines(image.split(":")[0] ?? image, image).slice(1, -1), out(`Status: Downloaded newer image for ${image}`)];
        case "create":
          return [cmd(`docker rm --force ${name}`), out(name), cmd(`docker run --detach --name ${name} ${image}`), out(fakeHex(`${name}${image}`, 64))];
        case "health":
          return [cmd(`docker inspect --format '{{.State.Status}}' ${name}`), out("running")];
        default:
          return genericLines(step);
      }
    },
    failure(_step, message) {
      return [err(`Error response from daemon: ${message}`, "error")];
    },
  };
}

/** `docker pull` of one reference with layer progress. */
export function pullScript(reference: string, known: boolean): OutputScript {
  const [repo = reference] = reference.split(/:(?=[^:/]+$)/);
  return {
    lines(step) {
      if (step.key === "collect") return [cmd(`docker pull ${reference}`), err(`${reference.split(":")[1] ?? "latest"}: Pulling from ${repo}`)];
      if (step.key === "pull") return known ? [err("Digest: sha256:" + fakeHex(reference, 64)), out(`Status: Image is up to date for ${reference}`)] : layerLines(repo, reference).slice(1, -1);
      if (step.key === "save") return known ? [] : [out(`Digest: sha256:${fakeHex(reference, 64)}`), out(`Status: Downloaded newer image for ${reference}`)];
      return genericLines(step);
    },
    failure(_step, message) {
      return [err(`Error response from daemon: pull access denied for ${repo}, repository does not exist or may require 'docker login' (simuliert)`, "error"), err(message, "error")];
    },
  };
}

/** `docker image rm` for the prune candidates and the reclaimed size. */
export function pruneScript(items: { reference: string; digest: string; sizeMb: number }[], hostName: string): OutputScript {
  return {
    lines(step) {
      if (step.key === "collect") return [cmd("docker image ls --filter dangling=true --quiet"), out(`${items.length} Kandidaten auf ${hostName}, keiner von einem Container genutzt`)];
      if (step.key === "stop") return items.flatMap((i) => [cmd(`docker image rm ${i.reference}`), out(`Untagged: ${i.reference}`), out(`Deleted: ${i.digest}`)]);
      if (step.key === "save") return [out(`Total reclaimed space: ${(items.reduce((n, i) => n + i.sizeMb, 0) / 1024).toFixed(2)}GB`)];
      return genericLines(step);
    },
    failure(_step, message) {
      return [err(message, "error")];
    },
  };
}

function genericLines(step: OutputStep): OutputSpec[] {
  return [out(`${step.label}${step.detail ? ` – ${step.detail}` : ""}`)];
}

/** Fallback for actions without a CLI counterpart (scans, backups, onboarding). */
export const genericScript: OutputScript = {
  lines: genericLines,
  failure: (_step, message) => [err(message, "error")],
};
