// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import type { RegistriesApi, Registry, RegistryDraft } from "@/domain/registries";
import { AdapterError } from "../api";

export function createDemoRegistries(changed: () => void): RegistriesApi {
  const rows = new Map<string, Registry>();
  const save = (id: string, input: RegistryDraft): Registry => {
    const previous = rows.get(id);
    const row: Registry = { id, url: input.url, description: input.description, registryType: input.registryType, enabled: input.enabled, insecure: input.insecure,
      awsRegion: input.awsRegion, repositoryNames: input.repositoryNames ?? [], hasCredential: input.clearCredential ? false : !!(input.token || input.awsSecretAccessKey || previous?.hasCredential) };
    rows.set(id, row); changed(); return structuredClone(row);
  };
  return {
    listContainerRegistries: async () => structuredClone([...rows.values()]),
    createContainerRegistry: async (input) => save(crypto.randomUUID(), input),
    updateContainerRegistry: async (id, input) => { if (!rows.has(id)) throw new AdapterError("not-found"); return save(id, input); },
    deleteContainerRegistry: async (id) => { rows.delete(id); changed(); },
    testContainerRegistry: async (id) => ({ ok: !!rows.get(id)?.hasCredential, checkedAt: new Date().toISOString(), ...(rows.get(id)?.hasCredential ? {} : { code: "registry.authentication-required" }) }),
  };
}
