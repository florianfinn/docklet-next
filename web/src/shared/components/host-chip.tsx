import type { Host } from "@/domain/model";
import { cn } from "@/shared/lib/utils";

export function HostChip({ host, className, compact }: { host: Host | undefined; className?: string; compact?: boolean }) {
  if (!host) return null;
  return (
    <span className={cn("inline-flex min-w-0 items-center gap-1.5 text-xs text-muted-foreground", className)}>
      <i aria-hidden className="size-2.5 shrink-0 rounded-[3px]" style={{ background: host.color }} />
      {!compact && <span className="truncate">{host.name}</span>}
      {host.status === "offline" && <span className="size-1.5 rounded-full bg-destructive" aria-hidden />}
    </span>
  );
}
