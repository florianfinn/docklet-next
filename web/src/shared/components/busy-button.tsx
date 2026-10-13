import { Loader2Icon } from "lucide-react";
import type { ComponentProps, MouseEvent, ReactNode } from "react";

import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";

interface BusyButtonProps extends Omit<ComponentProps<typeof Button>, "asChild"> {
  busy: boolean;
  /** Icon shown when idle; the spinner takes its place while busy. */
  icon?: ReactNode;
  /** Optional text while busy, e.g. "Wird neu gestartet …"; defaults to the children. */
  busyLabel?: ReactNode;
}

/**
 * Button for real execution: while busy it shows a spinner, sets aria-busy
 * and ignores clicks, but stays focusable (aria-disabled instead of disabled)
 * so keyboard focus does not jump away. Navigation buttons do not use it.
 */
export function BusyButton({ busy, icon, busyLabel, children, onClick, className, disabled, ...props }: BusyButtonProps) {
  const guarded = (e: MouseEvent<HTMLButtonElement>) => {
    if (busy) {
      e.preventDefault();
      return;
    }
    onClick?.(e);
  };
  return (
    <Button
      {...props}
      disabled={disabled}
      aria-busy={busy || undefined}
      aria-disabled={busy || undefined}
      data-busy={busy || undefined}
      onClick={guarded}
      className={cn("aria-busy:cursor-progress aria-busy:opacity-80", className)}
    >
      {busy ? <Loader2Icon data-icon="inline-start" className="animate-spin motion-reduce:animate-none" aria-hidden /> : icon}
      {busy && busyLabel ? busyLabel : children}
    </Button>
  );
}
