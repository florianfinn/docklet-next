// Source: docklet-next/issues/205
// Revision: be813fd8a9f61eaf99161d4141e5c324b5c34094
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original shared inline rejection state and accessible modal error view.

import { CircleAlert } from "lucide-react";
import { useLayoutEffect, useRef, useSyncExternalStore, type ReactNode } from "react";
import { Button } from "@/shared/ui/button";
import { DialogFooter } from "@/shared/ui/dialog";
import type { DialogCommand } from "./use-dialog-command";

export function DialogError({ feedback }: { feedback: DialogCommand }) {
  const error = useSyncExternalStore(feedback.subscribe, feedback.snapshot, feedback.snapshot);
  const element = useRef<HTMLDivElement>(null);
  const focused = useRef<typeof error>(null);
  useLayoutEffect(() => {
    if (!error || focused.current === error || !element.current) return;
    focused.current = error;
    element.current.scrollIntoView?.({ block: "nearest" });
    element.current.focus({ preventScroll: true });
  }, [error]);
  return error && <div role="alert" data-slot="dialog-error" tabIndex={-1} ref={element} className="flex min-w-0 items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive [overflow-wrap:anywhere]">
    <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
    <span className="min-w-0 flex-1">{error.message}</span>
    {error.options?.action && <Button type="button" size="sm" variant="outline" onClick={error.options.action.onClick}>{error.options.action.label}</Button>}
  </div>;
}

export function DialogCommandFooter({ feedback, children, className }: { feedback: DialogCommand; children: ReactNode; className?: string }) {
  return <div className="flex min-w-0 flex-col gap-3"><DialogError feedback={feedback} /><DialogFooter className={className}>{children}</DialogFooter></div>;
}
