// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original bilingual concrete binding refusal consumer.

import { useTranslations } from "use-intl";
import type { ImageBinding } from "@/data/http/update-binding";
import { IMAGE_BINDING_KEYS } from "@/data/wire-catalogue-update";
export function BindingRefusal({ binding }: { binding?: ImageBinding }) {
  const t = useTranslations(), check = useTranslations("containers.updateCheck");
  let reason = binding ? t(IMAGE_BINDING_KEYS[binding.reason], { service: binding.service ?? "", variable: binding.variable ?? "—" }) : check("generic");
  if (binding?.scope === "service" && !["variable-reference", "digest-pinned", "build-service", "shared-image-anchor"].includes(binding.reason)) reason = check("serviceReason", { service: binding.service ?? "", reason });
  return <p role="status" className="text-sm text-muted-foreground">{check("notOffered", { reason })}</p>;
}
