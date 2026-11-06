// Source: docklet-next/issues/356
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original role-scoped command palette using existing query consumers. M19 web consumer integration.

import { useTranslations } from "use-intl";
import { CommandGroup, CommandItem } from "@/shared/ui/command";
import { Badge } from "@/shared/ui/badge";
import { Spinner } from "@/shared/ui/spinner";
import { matchSearch, type SearchEntry } from "./search-model";
export function SearchGroup({ name, entries, query, select, loading, list }: { name: string; entries: SearchEntry[]; query: string; select: (to?: string, run?: () => void) => void; loading?: boolean; list?: string }) {
  const t = useTranslations("shell.search"), matches = matchSearch(entries, query);
  return <CommandGroup heading={name}>{loading && <div role="status" className="flex gap-2 p-2 text-sm"><Spinner />{t("loading")}</div>}{matches.map((item) => <CommandItem key={item.id} value={`${name}:${item.id}`} onSelect={() => select(item.to, item.run)}><span>{item.label}</span>{item.host && <Badge variant="secondary" className="ml-auto">{item.host}</Badge>}</CommandItem>)}{list && matches.length > 0 && <CommandItem value={`all:${name}`} onSelect={() => select(list)}>{t("showAll", { list: name })}</CommandItem>}</CommandGroup>;
}
