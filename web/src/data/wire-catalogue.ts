// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original total catalogue maps for rendered contract enums.

import type { MessageKeys, NestedKeyOf } from "use-intl";
import type { Messages } from "@/app/i18n/catalog";

export type CatalogueKey = MessageKeys<Messages, NestedKeyOf<Messages>>;

/** Unknown codes can come from older rows or newer agents; only own map entries are guarded. */
export function wireKey<const T extends Record<string, CatalogueKey>>(map: T, code: string | undefined): T[keyof T] | undefined {
  return code !== undefined && Object.hasOwn(map, code) ? map[code as keyof T] : undefined;
}

/** Retains existing scoped translators while the map checks the full catalogue path. */
export function scopedKey<const K extends string, const P extends string>(key: K, prefix: P): K extends `${P}${infer S}` ? S : never {
  return key.slice(prefix.length) as K extends `${P}${infer S}` ? S : never;
}
