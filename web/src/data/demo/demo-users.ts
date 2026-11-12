// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original in-memory user and personal-key contract simulation.

import type { ManagedUser, PersonalApiKey, UsersApi } from "../api";
import { AdapterError } from "../api";
import { AdapterOutcomeError } from "../capability";
import type { DemoDb } from "./demo-db";

export function createDemoUsers(db: DemoDb): UsersApi {
  let users: ManagedUser[] = [{ id: "demo-admin", username: db.settings.auth.local.username, isAdmin: true, assignments: [], requiresPasswordChange: false }];
  let keys: PersonalApiKey[] = [];
  const get = (id: string) => { const u = users.find((u) => u.id === id); if (!u) throw new AdapterError("not-found"); return u; };
  const guardLast = (u: ManagedUser) => { if (u.isAdmin && !u.disabledAt && users.filter((u) => u.isAdmin && !u.disabledAt).length <= 1) throw new AdapterError("last-admin"); };
  const local = () => { if (db.settings.auth.mode !== "local") throw new AdapterOutcomeError({ kind: "error", code: "access.admin-required", status: 403 }); };
  return {
    listUsers: async () => structuredClone(users),
    createUser: async (input) => {
      if (!input.username.trim() || users.some((u) => u.username === input.username)) throw new AdapterError("exists");
      const u: ManagedUser = { id: crypto.randomUUID(), username: input.username, isAdmin: false, assignments: [], requiresPasswordChange: db.settings.auth.mode === "local" };
      users.push(u); return structuredClone(u);
    },
    updateUser: async (id, input) => { const u = get(id); if (input.username) u.username = input.username; return structuredClone(u); },
    disableUser: async (id) => { const u = get(id); guardLast(u); u.disabledAt = new Date().toISOString(); },
    enableUser: async (id) => { delete get(id).disabledAt; },
    resetUserPassword: async (id) => { local(); get(id).requiresPasswordChange = true; },
    deleteUser: async (id) => { guardLast(get(id)); users = users.filter((u) => u.id !== id); },
    setUserAdministrator: async (id, isAdmin) => {
      if (id === "demo-admin") throw new AdapterError("last-admin");
      const u = get(id); if (!isAdmin) guardLast(u); u.isAdmin = isAdmin;
    },
    setUserAppAssignments: async (id, assignments) => { get(id).assignments = structuredClone(assignments); },
    listMyApiKeys: async () => { local(); return structuredClone(keys); },
    createMyApiKey: async (input) => {
      local();
      const key = `example_${crypto.randomUUID()}`;
      const row: PersonalApiKey = { id: crypto.randomUUID(), name: input.name, expiresAt: input.expiresAt, keyPrefix: key.slice(0, 12), createdAt: new Date().toISOString() };
      keys.push(row); return { ...row, key };
    },
    deleteMyApiKey: async (id) => { local(); keys = keys.filter((k) => k.id !== id); },
  };
}
