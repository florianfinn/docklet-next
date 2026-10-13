import type { HealthForm } from "./model";

/** The parts of a service the suggestions read. */
export interface PresetSource {
  image: string;
  ports: readonly { container: string }[];
}

/**
 * Healthcheck suggestions for the Easy editor. App presets only match official
 * images whose tool ships in that image; HTTP presets are generic and name the
 * tool the image must contain. Nothing here is tested against the container.
 */
export interface HealthPreset {
  id: "postgres" | "mariadb" | "mysql" | "redis" | "valkey" | "http-curl" | "http-wget";
  /** Exec form (CMD) as arguments; shell form (CMD-SHELL) as one command line. */
  form: Exclude<HealthForm, "string">;
  test: string[];
  interval: string;
  timeout: string;
  retries: number;
  startPeriod?: string;
  /** Programs the image must provide. */
  requires: string[];
  /** True when the image name matched; false for generic suggestions. */
  matched: boolean;
}

/** Last path segment of the repository without tag or digest: "docker.io/library/postgres:17" → "postgres". */
export function imageName(image: string): string {
  const ref = image.trim().split("@")[0] ?? "";
  const last = ref.split("/").pop() ?? "";
  return last.split(":")[0]?.toLowerCase() ?? "";
}

/** Official images only: library images or the vendor's own repository. */
function official(image: string, name: string, vendors: string[] = []): boolean {
  const ref = image.trim().toLowerCase().split(/[:@]/)[0] ?? "";
  const repo = ref.replace(/^docker\.io\//, "").replace(/^library\//, "");
  return repo === name || vendors.includes(repo);
}

const BASE = { interval: "30s", timeout: "5s", retries: 3 } as const;

/** First published or declared container port, else 80 (stated as an assumption in the UI). */
export function httpPort(service: Pick<PresetSource, "ports">): { port: string; known: boolean } {
  const first = service.ports.find((p) => p.container.trim());
  const port = first?.container.trim().split("/")[0]?.split("-")[0];
  return port && /^\d+$/.test(port) ? { port, known: true } : { port: "80", known: false };
}

/** One or two suggestions: a matching app check, or the two generic HTTP checks. */
export function healthPresets(service: PresetSource): HealthPreset[] {
  const img = service.image;
  const app: HealthPreset | undefined = official(img, "postgres")
    ? { id: "postgres", form: "CMD-SHELL", test: ["pg_isready -U \"$${POSTGRES_USER:-postgres}\""], ...BASE, startPeriod: "20s", requires: ["pg_isready"], matched: true }
    : official(img, "mariadb")
      ? { id: "mariadb", form: "CMD", test: ["healthcheck.sh", "--connect", "--innodb_initialized"], ...BASE, startPeriod: "30s", requires: ["healthcheck.sh"], matched: true }
      : official(img, "mysql")
        ? { id: "mysql", form: "CMD", test: ["mysqladmin", "ping", "-h", "localhost"], ...BASE, startPeriod: "30s", requires: ["mysqladmin"], matched: true }
        : official(img, "redis")
          ? { id: "redis", form: "CMD", test: ["redis-cli", "ping"], ...BASE, requires: ["redis-cli"], matched: true }
          : official(img, "valkey", ["valkey/valkey"])
            ? { id: "valkey", form: "CMD", test: ["valkey-cli", "ping"], ...BASE, requires: ["valkey-cli"], matched: true }
            : undefined;
  if (app) return [app];
  const { port } = httpPort(service);
  const url = `http://localhost:${port}/`;
  return [
    { id: "http-curl", form: "CMD-SHELL", test: [`curl -fsS ${url} || exit 1`], ...BASE, requires: ["curl"], matched: false },
    { id: "http-wget", form: "CMD-SHELL", test: [`wget -q --spider ${url} || exit 1`], ...BASE, requires: ["wget"], matched: false },
  ];
}

/** Exec arguments as an editable JSON list: ["redis-cli", "ping"]. */
export function formatArgs(args: readonly string[]): string {
  return `[${args.map((a) => JSON.stringify(a)).join(", ")}]`;
}

/** Parses the JSON list the CMD field holds; undefined unless it is an array of strings. */
export function parseArgs(text: string): string[] | undefined {
  try {
    const v: unknown = JSON.parse(text);
    return Array.isArray(v) && v.every((a) => typeof a === "string") ? v : undefined;
  } catch {
    return undefined;
  }
}

/** What the Easy form shows for a preset: the JSON list for CMD, the command line otherwise. */
export function presetCommand(p: HealthPreset): string {
  return p.form === "CMD" ? formatArgs(p.test) : p.test.join(" ");
}

/** True when the service's current test already is this preset's test. */
export function sameTest(h: { form: HealthForm; test: string; args: readonly string[] }, p: HealthPreset): boolean {
  if (p.form === "CMD") return h.form === "CMD" && formatArgs(h.args) === formatArgs(p.test);
  return h.form !== "CMD" && h.test.trim() === p.test.join(" ");
}
