import { Unplug } from "lucide-react";
import { useLocale } from "use-intl";

import { Alert, AlertDescription, AlertTitle } from "@/shared/ui/alert";

import { deBackend } from "./messages/de";
import { enBackend } from "./messages/en";
import { useAdapter } from "./use-adapter";

export function BackendNotice() {
  const adapter = useAdapter();
  const messages = useLocale() === "de" ? deBackend : enBackend;
  if (adapter.kind !== "disconnected") return null;
  return (
    <Alert className="mb-5">
      <Unplug />
      <AlertTitle>{messages.title}</AlertTitle>
      <AlertDescription>{messages.hint}</AlertDescription>
    </Alert>
  );
}
