// Source: docklet-next/issues/503
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original proven Compose path link; display ownership is never used.

import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { useContainers, useStacks } from "@/data/queries";
import { manualImportStack } from "./suggestions";
import type { Instance } from "./types";

export function ManualImportHint({ instance }: { instance: Instance }) {
  const t = useTranslations("proxyContract.ui"), containers = useContainers(), stacks = useStacks();
  const stack = manualImportStack(instance, containers.data ?? [], stacks.data ?? []);
  return <p className="text-sm">{t("manualImport")}{stack && <> <Link className="text-primary underline" to={`/stacks/${encodeURIComponent(stack.id)}?tab=editor`}>{t("stackEditor")}</Link></>}</p>;
}
