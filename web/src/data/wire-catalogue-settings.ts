// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";

type Schema = components["schemas"];

export const NOTIFICATION_CODE_KEYS = {
  "notification.config-invalid": "settings.codes.notification.config-invalid",
  "notification.hub-only": "settings.codes.notification.hub-only",
  "notification.dispatch-invalid": "settings.codes.notification.dispatch-invalid",
  "notification.dispatch-unsupported": "settings.codes.notification.dispatch-unsupported",
  "notification.dispatch-stale": "settings.codes.notification.dispatch-stale",
  "notification.test-busy": "settings.codes.notification.test-busy",
} satisfies Record<NonNullable<
  | Schema["NotificationProblem"]["code"]
>, CatalogueKey>;

export const AUTOHEAL_CODE_KEYS = {
  "autoheal.settings-invalid": "settings.codes.autoheal.settings-invalid",
  "autoheal.settings-env-managed": "settings.codes.autoheal.settings-env-managed",
  "settings.key-managed": "settings.codes.settings.key-managed",
} satisfies Record<NonNullable<
  | Schema["AutohealProblem"]["code"]
>, CatalogueKey>;

export const NOTIFICATION_TEST_KEYS = {
  "notification.test-sent": "settings.codes.notification.test-sent",
  "notification.test-failed": "settings.codes.notification.test-failed",
} satisfies Record<NonNullable<
  | Schema["NotificationTestResponse"]["code"]
>, CatalogueKey>;

export const DELIVERY_REASON_KEYS = {
  "delivery-unreachable": "settings.codes.delivery-unreachable",
  "delivery-rejected": "settings.codes.delivery-rejected",
  "delivery-auth": "settings.codes.delivery-auth",
  "delivery-invalid": "settings.codes.delivery-invalid",
} satisfies Record<NonNullable<
  | Schema["NotificationTestResponse"]["reason"]
>, CatalogueKey>;
