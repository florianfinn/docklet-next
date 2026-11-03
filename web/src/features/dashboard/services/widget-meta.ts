// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original service widget accents and error keys for issues #332 and #333.

import type { ServiceKind, WidgetError, WidgetErrorCode } from "@/domain/service-widgets";

/** Accent colour per kind for tints. */
export const SERVICE_COLOR: Record<ServiceKind, string> = { plex: "#e5a00d", minecraft: "#6aa84f", overseerr: "#8b5cf6" };

type ErrorSuffix = WidgetErrorCode extends `widget.${infer S}` ? S : never;

/** Key below dashboard.services.errors for a hub widget code. */
export function widgetErrorKey(error: WidgetError): `errors.${ErrorSuffix}` {
  return `errors.${error.code.slice("widget.".length) as ErrorSuffix}`;
}
