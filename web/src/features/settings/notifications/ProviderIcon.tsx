// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original channel icon mapping.

import { BellRing, Mail, MessageCircle, Webhook } from "lucide-react";

export function ProviderIcon({ provider }: { provider: string }) {
  if (provider === "email") return <Mail aria-hidden />;
  if (provider === "generic" || provider === "googlechat") return <Webhook aria-hidden />;
  if (provider === "ntfy" || provider === "pushover" || provider === "gotify") return <BellRing aria-hidden />;
  return <MessageCircle aria-hidden />;
}
