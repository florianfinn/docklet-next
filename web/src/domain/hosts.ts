import type { Host, Tone } from "./model";

/** German-aware, case-insensitive, numeric ("Host 2" before "Host 10"); shared by every sort in the UI. */
export const COLLATOR = new Intl.Collator("de", { sensitivity: "base", numeric: true });

/** Hosts in one deliberate order everywhere: by name, ties by id. */
export function sortHosts<T extends Pick<Host, "id" | "name">>(hosts: readonly T[]): T[] {
  return hosts.toSorted((a, b) => COLLATOR.compare(a.name, b.name) || COLLATOR.compare(a.id, b.id));
}

/** Empty selection means "all hosts"; unknown ids are ignored. */
export function isHostVisible(selection: readonly string[], hostId: string): boolean {
  return selection.length === 0 || selection.includes(hostId);
}

export function filterByHosts<T extends { hostId: string }>(items: readonly T[], selection: readonly string[]): T[] {
  if (selection.length === 0) return [...items];
  const set = new Set(selection);
  return items.filter((item) => set.has(item.hostId));
}

export function toggleHost(selection: readonly string[], hostId: string, allIds: readonly string[]): string[] {
  const current = selection.length === 0 ? [...allIds] : [...selection];
  const next = current.includes(hostId) ? current.filter((id) => id !== hostId) : [...current, hostId];
  // Selecting every host again, or nothing, collapses back to the default "all".
  if (next.length === 0 || next.length === allIds.length) return [];
  return allIds.filter((id) => next.includes(id));
}

export function hostTone(host: Host | undefined): Tone {
  if (!host) return "gray";
  // A disconnected host is a problem (red), like everywhere else in the fleet view.
  return host.status === "online" ? "ok" : host.status === "pending" ? "info" : "err";
}

const LABEL_TONES: Record<string, Tone> = {
  produktiv: "ok",
  media: "violet",
  spiele: "orange",
  intern: "sky",
  test: "warn",
  dokumente: "teal",
  "smart-home": "cyan",
  netz: "indigo",
  monitoring: "pink",
};
const LABEL_CYCLE: Tone[] = ["pink", "lime", "info", "accent", "teal", "rose"];

export function labelTone(label: string): Tone {
  const known = LABEL_TONES[label];
  if (known) return known;
  let h = 0;
  for (const ch of label) h = (h * 31 + ch.charCodeAt(0)) % 997;
  return LABEL_CYCLE[h % LABEL_CYCLE.length] ?? "gray";
}

/** Stable palette defaults survive reloads and do not depend on list order. */
export const HOST_COLORS = ["#8b5cf6", "#22c55e", "#eab308", "#ec4899", "#06b6d4", "#ef4444", "#84cc16", "#6366f1"];
export function defaultHostColor(id: string): string {
  let hash = 0;
  for (const ch of id) hash = (hash * 31 + ch.charCodeAt(0)) >>> 0;
  return HOST_COLORS[hash % HOST_COLORS.length]!;
}
