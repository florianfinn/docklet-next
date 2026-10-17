// Field validators shared by onboarding, setup and settings. They return a
// message key (resolved by the caller's namespace) or undefined when valid.

export type ValidationKey =
  | "required"
  | "absolutePath"
  | "pathChars"
  | "hostAddress"
  | "port"
  | "url"
  | "httpsRecommended"
  | "id"
  | "idRoot"
  | "hostName"
  | "duplicate"
  | "volumeName"
  | "email"
  | "minLength"
  | "mismatch"
  | "headerName"
  | "hubAddress";

export function required(value: string): ValidationKey | undefined {
  return value.trim() ? undefined : "required";
}

export function absolutePath(value: string): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return "required";
  if (!v.startsWith("/")) return "absolutePath";
  if (/[\s"'`$\\]/.test(v) || v.includes("//") || v.split("/").some((part) => part === ".." || part === ".")) return "pathChars";
  return undefined;
}

/** Normalised absolute path without trailing slash, for comparisons. */
export function normalizePath(value: string): string {
  const v = value.trim().replace(/\/+$/, "");
  return v || "/";
}

export function isInside(child: string, parent: string): boolean {
  const c = normalizePath(child);
  const p = normalizePath(parent);
  return c === p || c.startsWith(p === "/" ? "/" : `${p}/`);
}

export const AGENT_DIR_NAME = "docklet-agent";

/** Default agent install directory: its own folder directly in the project directory. */
export function defaultInstallDir(projectDir: string): string {
  const p = normalizePath(projectDir);
  return `${p === "/" ? "" : p}/${AGENT_DIR_NAME}`;
}

export type InstallDirProblem = "sameDir" | "containsProject" | "insideStack";

/**
 * Where the agent may live relative to the project directory. Its own folder
 * directly inside the project directory is fine (stack discovery skips it by
 * label). Not allowed: the project directory itself (the agent's compose.yaml
 * would take the root), a parent of it, or a folder inside a stack folder.
 */
export function installDirProblem(projectDir: string, installDir: string): InstallDirProblem | undefined {
  const p = normalizePath(projectDir);
  const i = normalizePath(installDir);
  if (p === i) return "sameDir";
  if (isInside(p, i)) return "containsProject";
  if (isInside(i, p)) {
    const rest = i.slice(p === "/" ? 1 : p.length + 1);
    if (rest.includes("/")) return "insideStack";
  }
  return undefined;
}

const HOSTNAME = /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)(\.[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)*$/i;
const IPV4 = /^(25[0-5]|2[0-4]\d|1?\d?\d)(\.(25[0-5]|2[0-4]\d|1?\d?\d)){3}$/;

export function hostAddress(value: string): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return "required";
  return IPV4.test(v) || HOSTNAME.test(v) ? undefined : "hostAddress";
}

export function port(value: string): ValidationKey | undefined {
  if (!/^\d{1,5}$/.test(value.trim())) return "port";
  const n = Number(value);
  return n >= 1 && n <= 65535 ? undefined : "port";
}

/**
 * Hub address reachable from agents: an IP or host name with optional port,
 * or an http(s) URL. Internal LAN/VPN addresses are the normal case.
 */
export function hubAddress(value: string): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return "required";
  if (/^https?:\/\//i.test(v)) return url(v) ? "hubAddress" : undefined;
  const [host = "", portPart, ...rest] = v.split(":");
  if (rest.length) return "hubAddress";
  if (portPart !== undefined && port(portPart)) return "hubAddress";
  return IPV4.test(host) || HOSTNAME.test(host) ? undefined : "hubAddress";
}

/** http:// is assumed for bare addresses; agents and links use the URL form. */
export function hubAddressUrl(value: string): string {
  const v = value.trim().replace(/\/+$/, "");
  return /^https?:\/\//i.test(v) ? v : `http://${v}`;
}

export function url(value: string): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return "required";
  try {
    const u = new URL(v);
    if (u.protocol !== "https:" && u.protocol !== "http:") return "url";
    return u.hostname ? undefined : "url";
  } catch {
    return "url";
  }
}

/** Optional UID/GID: empty is allowed (installer proposes one). */
export function optionalId(value: string): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return undefined;
  if (!/^\d{1,5}$/.test(v) || Number(v) > 65534) return "id";
  return undefined;
}

export function hostName(value: string, taken: readonly string[]): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return "required";
  if (!/^[\p{L}\d][\p{L}\d ._-]{0,30}$/u.test(v)) return "hostName";
  return taken.some((t) => t.toLowerCase() === v.toLowerCase()) ? "duplicate" : undefined;
}

/** Docker volume names: start alphanumeric, then [a-zA-Z0-9_.-]. */
export function volumeName(value: string, taken: readonly string[]): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return "required";
  if (!/^[a-zA-Z0-9][a-zA-Z0-9_.-]{1,63}$/.test(v)) return "volumeName";
  return taken.includes(v) ? "duplicate" : undefined;
}

export function email(value: string): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return "required";
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v) ? undefined : "email";
}

export function headerName(value: string): ValidationKey | undefined {
  const v = value.trim();
  if (!v) return "required";
  return /^[A-Za-z0-9-]{2,64}$/.test(v) ? undefined : "headerName";
}

export function slugify(value: string): string {
  return (
    value
      .normalize("NFKD")
      .replace(/[̀-ͯ]/g, "")
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "host"
  );
}
