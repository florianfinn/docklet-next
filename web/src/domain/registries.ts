// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

/** Public registry configuration. Credential identities and secrets are write-only. */
export interface Registry {
  id: string;
  url: string;
  description: string;
  registryType: string;
  enabled: boolean;
  insecure: boolean;
  hasCredential: boolean;
  awsRegion?: string;
  repositoryNames: string[];
}
export interface RegistryDraft {
  url: string;
  description: string;
  registryType: "generic" | "ecr";
  enabled: boolean;
  insecure: boolean;
  awsRegion?: string;
  repositoryNames?: string[];
  username?: string;
  token?: string;
  awsAccessKeyId?: string;
  awsSecretAccessKey?: string;
  clearCredential?: boolean;
}
export interface RegistryTest { ok: boolean; checkedAt: string; code?: string }
export interface RegistriesApi {
  listContainerRegistries(): Promise<Registry[]>;
  createContainerRegistry(input: RegistryDraft): Promise<Registry>;
  updateContainerRegistry(id: string, input: RegistryDraft): Promise<Registry>;
  deleteContainerRegistry(id: string): Promise<void>;
  testContainerRegistry(id: string): Promise<RegistryTest>;
}

/** Selection changes the reference host; the executor chooses that host's saved credentials. */
export function registryHost(url: string): string | undefined {
  try {
    const parsed = new URL(url.includes("://") ? url : `https://${url}`);
    if (!["https:", "http:"].includes(parsed.protocol) || parsed.username || parsed.password || !parsed.hostname) return undefined;
    return ["docker.io", "index.docker.io", "registry-1.docker.io"].includes(parsed.hostname) ? "docker.io" : parsed.host;
  } catch { return undefined; }
}
export function registryReference(reference: string, host?: string): string {
  const value = reference.trim();
  if (!host) return value;
  const slash = value.indexOf("/");
  const first = slash < 0 ? "" : value.slice(0, slash);
  const path = first && (/[.:]/.test(first) || first === "localhost") ? value.slice(slash + 1) : value;
  return host === "docker.io" ? `docker.io/${path.includes("/") ? path : `library/${path}`}` : `${host}/${path}`;
}
