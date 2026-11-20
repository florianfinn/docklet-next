// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

import { useTranslations } from "use-intl";
import type { Stack } from "@/domain/model";

export function useStackDefinitionHint(stack: Stack) {
 const t = useTranslations("common.runtimePeers");
 if (stack.definitionReason === "compose.env-file-missing") return stack.definitionFile ? t("markerFile", { file: stack.definitionFile }) : t("markerMissing");
 if (stack.definitionReason === "compose.selection-unsupported") return t("selection");
 return t("definitionInvalid");
}
