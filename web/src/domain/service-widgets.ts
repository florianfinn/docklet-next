// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original service widget view model for issues #332 and #333.

/** One widget per kind; the hub fetches, the browser only renders. */
export const SERVICE_KINDS = ["plex", "minecraft", "overseerr"] as const;
export type ServiceKind = (typeof SERVICE_KINDS)[number];

export const WIDGET_ERROR_CODES = [
  "widget.not-configured", "widget.disabled", "widget.unreachable", "widget.timeout", "widget.auth-failed", "widget.bad-response",
  "widget.redirect-refused", "widget.tls-failed", "widget.credential-unavailable", "widget.invalid-config", "widget.persistence-failed",
] as const;
export type WidgetErrorCode = (typeof WIDGET_ERROR_CODES)[number];

export interface CertificatePin {
  hostPort: string;
  sha256: string;
}

export interface WidgetError {
  code: WidgetErrorCode;
  /** Server certificate of a failed TLS verification, offered for pinning (test action only). */
  certificate?: CertificatePin;
}

export interface WidgetSummary {
  kind: ServiceKind;
  configured: boolean;
  enabled: boolean;
  openUrl: string;
  /** Inventory id of the linked stack. */
  stackId?: string;
}

export interface PlexSession {
  user: string;
  title: string;
  subtitle: string;
  device: string;
  /** 0–100 */
  progress: number;
  quality: string;
  transcode: boolean;
}

export interface MediaRequest {
  id: number;
  title: string;
  mediaType: "movie" | "tv";
  requestedBy: string;
  createdAt: string;
}

export interface WidgetData {
  plex?: { streams: number; transcodes: number; sessions: PlexSession[] };
  overseerr?: { pending: number; approved: number; available: number; requests: MediaRequest[] };
  minecraft?: { online: number; max: number; motd: string; version: string; players: string[] };
}

export interface WidgetResult extends WidgetData {
  kind: ServiceKind;
  /** Last good data is kept while a later fetch fails; it is then stale. */
  stale: boolean;
  ageSeconds: number;
  fetchedAt?: string;
  error?: WidgetError;
}

export interface WidgetConfig {
  kind: ServiceKind;
  enabled: boolean;
  address: string;
  hasCredential: boolean;
  openUrl: string;
  stackId?: string;
  tlsPin?: CertificatePin;
  revision: number;
}

export type CredentialAction = "keep" | "replace" | "clear";

export interface WidgetConfigInput {
  enabled: boolean;
  address: string;
  credentialAction: CredentialAction;
  /** Only with `replace`; never read back. */
  credential?: string;
  openUrl: string;
  stackId?: string;
  tlsPin?: CertificatePin;
}

export interface ServiceWidgetsApi {
  /** Every signed-in user. */
  listServiceWidgets(): Promise<WidgetSummary[]>;
  getServiceWidget(kind: ServiceKind): Promise<WidgetResult>;
  /** Administrators only. */
  getServiceWidgetConfig(kind: ServiceKind): Promise<WidgetConfig>;
  saveServiceWidgetConfig(kind: ServiceKind, input: WidgetConfigInput): Promise<WidgetConfig>;
  /** Fetches once with the draft (or the stored credential) and writes nothing. */
  testServiceWidget(kind: ServiceKind, input: WidgetConfigInput): Promise<WidgetResult>;
  decideMediaRequest(id: number, decision: "approve" | "decline"): Promise<void>;
}

/** Credential names per kind; Minecraft has none. */
export const CREDENTIAL_KIND: Record<ServiceKind, "token" | "apiKey" | undefined> = { plex: "token", overseerr: "apiKey", minecraft: undefined };

export function isWidgetErrorCode(value: unknown): value is WidgetErrorCode {
  return (WIDGET_ERROR_CODES as readonly unknown[]).includes(value);
}
