// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original in-memory service widgets with synthetic data for issues #332 and #333.

import { SERVICE_KINDS, type MediaRequest, type ServiceKind, type ServiceWidgetsApi, type WidgetConfig, type WidgetConfigInput, type WidgetErrorCode, type WidgetResult } from "@/domain/service-widgets";

import { AdapterOutcomeError } from "../capability";

/** Example addresses of the demo; nothing leaves the browser. */
export const DEMO_WIDGET_ADDRESS: Record<ServiceKind, string> = { plex: "http://192.0.2.21:32400", minecraft: "192.0.2.22:25565", overseerr: "http://192.0.2.23:5055" };

const SESSIONS = [
  { user: "Mara", title: "Nordlicht", subtitle: "Staffel 2 · Folge 4", quality: "1080p", transcode: false, device: "Wohnzimmer-TV", durationMin: 48, offset: 21 },
  { user: "Jonas", title: "Die Brücke am Fluss", subtitle: "Film · 2019", quality: "4K → 1080p", transcode: true, device: "Tablet", durationMin: 112, offset: 64 },
  { user: "Ida", title: "Klangwelten", subtitle: "Album · Titel 7", quality: "FLAC", transcode: false, device: "Küche", durationMin: 5, offset: 2 },
];
const REQUESTS: (Omit<MediaRequest, "createdAt"> & { ageHours: number })[] = [
  { id: 101, title: "Sternenfahrer", mediaType: "movie", requestedBy: "Jonas", ageHours: 2 },
  { id: 102, title: "Küstenwache", mediaType: "tv", requestedBy: "Mara", ageHours: 9 },
  { id: 103, title: "Das stille Tal", mediaType: "movie", requestedBy: "Ida", ageHours: 26 },
];

const reject = (code: WidgetErrorCode, status = 400) => new AdapterOutcomeError({ kind: "error", code, status });

/** Same rules as the hub: an address containing "fail" or "fehler" is unreachable. */
export function createDemoServiceWidgets(now: () => number = Date.now): ServiceWidgetsApi & { reset(): void } {
  let configs: Record<ServiceKind, WidgetConfig & { credential?: string }>;
  let pending: typeof REQUESTS;
  let approved = 4, available = 11;
  function reset() {
    configs = Object.fromEntries(SERVICE_KINDS.map((kind) => [kind, { kind, enabled: true, address: DEMO_WIDGET_ADDRESS[kind], hasCredential: kind !== "minecraft", credential: kind === "minecraft" ? undefined : "demo", openUrl: "", revision: 1 }])) as typeof configs;
    pending = structuredClone(REQUESTS);
    approved = 4; available = 11;
  }
  reset();
  const fails = (address: string) => /fail|fehler/i.test(address);
  function data(kind: ServiceKind, address: string): WidgetResult {
    const base = { kind, stale: false, ageSeconds: 0, fetchedAt: new Date(now()).toISOString() };
    if (!address.trim()) return { kind, stale: false, ageSeconds: 0, error: { code: "widget.not-configured" } };
    if (fails(address)) return { kind, stale: false, ageSeconds: 0, error: { code: "widget.unreachable" } };
    if (kind === "plex") {
      const sessions = SESSIONS.map((s) => ({ user: s.user, title: s.title, subtitle: s.subtitle, device: s.device, quality: s.quality, transcode: s.transcode, progress: Math.round((((now() / 60_000 + s.offset) % s.durationMin) / s.durationMin) * 100) }));
      return { ...base, plex: { streams: sessions.length, transcodes: sessions.filter((s) => s.transcode).length, sessions } };
    }
    if (kind === "minecraft") return { ...base, minecraft: { online: 4, max: 20, motd: "Werkstatt-Welt (Demo)", version: "1.21.8", players: ["Erzsucherin", "Blockbauer42", "Nachteule", "Kiesel_7"] } };
    const requests = pending.map(({ ageHours, ...r }) => ({ ...r, createdAt: new Date(now() - ageHours * 3_600_000).toISOString() }));
    return { ...base, overseerr: { pending: requests.length, approved, available, requests } };
  }
  const view = (c: WidgetConfig & { credential?: string }): WidgetConfig => {
    const { credential: _hidden, ...rest } = c;
    return structuredClone(rest);
  };
  function validate(kind: ServiceKind, input: WidgetConfigInput) {
    const address = input.address.trim();
    const ok = kind === "minecraft" ? /^[^\s/:]+(:\d{1,5})?$/.test(address) : /^https?:\/\/[^\s/@?#]+(\/[^\s?#]*)?$/.test(address);
    if (address && !ok) throw reject("widget.invalid-config");
    if (input.openUrl.trim() && !/^https?:\/\/\S+$/.test(input.openUrl.trim())) throw reject("widget.invalid-config");
  }
  return {
    reset,
    listServiceWidgets: async () => SERVICE_KINDS.map((kind) => ({ kind, configured: !!configs[kind].address, enabled: configs[kind].enabled, openUrl: configs[kind].openUrl, ...(configs[kind].stackId ? { stackId: configs[kind].stackId } : {}) })),
    async getServiceWidget(kind) {
      const c = configs[kind];
      if (!c.address) return { kind, stale: false, ageSeconds: 0, error: { code: "widget.not-configured" } };
      if (!c.enabled) return { kind, stale: false, ageSeconds: 0, error: { code: "widget.disabled" } };
      return data(kind, c.address);
    },
    getServiceWidgetConfig: async (kind) => view(configs[kind]),
    async saveServiceWidgetConfig(kind, input) {
      validate(kind, input);
      const prior = configs[kind];
      const credential = input.credentialAction === "replace" ? input.credential : input.credentialAction === "clear" ? undefined : prior.credential;
      configs[kind] = { kind, enabled: input.enabled, address: input.address.trim(), openUrl: input.openUrl.trim(), credential, hasCredential: !!credential, revision: prior.revision + 1, ...(input.stackId ? { stackId: input.stackId } : {}), ...(input.tlsPin ? { tlsPin: input.tlsPin } : {}) };
      return view(configs[kind]);
    },
    async testServiceWidget(kind, input) {
      validate(kind, input);
      return data(kind, input.address);
    },
    async decideMediaRequest(id, decision) {
      if (!pending.some((r) => r.id === id)) throw reject("widget.bad-response");
      pending = pending.filter((r) => r.id !== id);
      if (decision === "approve") approved++;
    },
  };
}
