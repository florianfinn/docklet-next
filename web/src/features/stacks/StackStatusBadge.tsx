import { Loader2 } from "lucide-react";
import { useTranslations } from "use-intl";

import type { Host, Stack } from "@/domain/model";
import { STACK_STATUS_TONE, stackStatus } from "@/domain/status";
import { ToneBadge, ToneDot } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";

export function StackStatusBadge({ stack, host, className }: { stack: Stack; host: Host | undefined; className?: string }) {
  const t = useTranslations("common.status");
  const status = stackStatus(stack, host);
  const tone = STACK_STATUS_TONE[status];
  return (
    <ToneBadge tone={tone} className={cn("min-w-24 justify-start", className)}>
      {status === "busy" ? <Loader2 className="animate-spin" aria-hidden /> : <ToneDot tone={tone} pulse={status === "problem"} className="size-1.5" />}
      {t(status)}
    </ToneBadge>
  );
}
