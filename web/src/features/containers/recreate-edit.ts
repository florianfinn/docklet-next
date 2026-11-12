// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original partial edit mapping and per-intent idempotency ownership.

import { commandUUID, intentFingerprint } from "@/data/intent-key";
import type { components } from "@/api/generated/schema";
export type EditConfig = components["schemas"]["ContainerEditConfig"];
export type ContainerEdit = components["schemas"]["ContainerRecreateRequest"]["edit"];
export interface RecreateDraft { name: string; image: string; restart: string; ports: string; environment: string }

export function recreateDraft(config: EditConfig): RecreateDraft {
  return { name: config.name, image: config.image, restart: config.hostConfig.restartPolicy.name ?? "no",
    ports: JSON.stringify(config.hostConfig.portBindings ?? {}, null, 2), environment: (config.environment ?? []).join("\n") };
}

function portBindings(text: string): NonNullable<ContainerEdit["hostConfig"]>["portBindings"] {
  const value: unknown = JSON.parse(text);
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("invalid-ports");
  for (const [port, entries] of Object.entries(value)) {
    if (!/^\d{1,5}\/(tcp|udp|sctp)$/.test(port) || Number(port.split("/")[0]) < 1 || Number(port.split("/")[0]) > 65535 ||
      (entries !== null && (!Array.isArray(entries) || !entries.every((entry: unknown) => {
        if (!entry || typeof entry !== "object" || Array.isArray(entry)) return false;
        return Object.entries(entry).every(([key, item]) => ["hostIp", "hostPort"].includes(key) && typeof item === "string");
      })))) throw new Error("invalid-ports");
  }
  return value as NonNullable<ContainerEdit["hostConfig"]>["portBindings"];
}

/** Omit untouched sections so host configuration outside the form survives. */
export function recreateEdit(initial: RecreateDraft, draft: RecreateDraft): ContainerEdit {
  const edit: ContainerEdit = { image: draft.image.trim() };
  if (draft.name !== initial.name) edit.name = draft.name.trim();
  if (draft.environment !== initial.environment) edit.environment = draft.environment.split("\n").filter((line) => line !== "");
  if (draft.restart !== initial.restart) edit.hostConfig = { restartPolicy: { name: draft.restart } };
  if (draft.ports !== initial.ports) edit.hostConfig = { ...edit.hostConfig, portBindings: portBindings(draft.ports) };
  return edit;
}

/** A retried submitted edit keeps its key; a changed submitted edit owns a new key. */
export function recreateIntent(uuid: () => string = commandUUID) {
  let previous: { edit: string; key: string } | undefined;
  return (edit: ContainerEdit) => {
    const fingerprint = intentFingerprint(edit);
    if (previous?.edit !== fingerprint) previous = { edit: fingerprint, key: uuid() };
    return previous.key;
  };
}
