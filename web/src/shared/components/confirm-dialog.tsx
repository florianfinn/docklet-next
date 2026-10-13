import { CircleAlert, Loader2Icon } from "lucide-react";
import { useRef, useState, type MouseEvent, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/shared/ui/alert-dialog";
import { useAdapterText } from "@/data/use-adapter-text";
import { buttonVariants } from "@/shared/ui/button";
import { cn } from "@/shared/lib/utils";

interface ConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: ReactNode;
  confirmLabel: string;
  /**
   * A synchronous confirm closes the dialog at once. A returned promise keeps
   * it open and busy until it settles: `false` or a rejection keep it open
   * with an error so the user can retry or cancel; any other value closes it.
   */
  onConfirm: () => unknown;
  destructive?: boolean;
  children?: ReactNode;
}

const isPromise = (v: unknown): v is Promise<unknown> => !!v && typeof (v as Promise<unknown>).then === "function";

/** Confirmation for destructive or draft-discarding actions; Escape and Cancel keep everything. */
export function ConfirmDialog({ open, onOpenChange, title, description, confirmLabel, onConfirm, destructive, children }: ConfirmDialogProps) {
  const t = useTranslations("common.actions");
  const dt = useAdapterText();
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inFlight = useRef(false);

  const change = (next: boolean) => {
    if (inFlight.current) return;
    setError(null);
    onOpenChange(next);
  };

  const confirm = (e: MouseEvent<HTMLButtonElement>) => {
    if (inFlight.current) {
      e.preventDefault();
      return;
    }
    const result = onConfirm();
    if (!isPromise(result)) return;
    e.preventDefault();
    inFlight.current = true;
    setPending(true);
    setError(null);
    result
      .then(
        (value) => {
          if (value === false) setError(t("confirmFailed"));
          else {
            inFlight.current = false;
            onOpenChange(false);
          }
        },
        (err: unknown) => setError(err instanceof Error && err.message ? dt(err.message) : t("confirmFailed")),
      )
      .finally(() => {
        inFlight.current = false;
        setPending(false);
      });
  };

  return (
    <AlertDialog open={open} onOpenChange={change}>
      <AlertDialogContent onEscapeKeyDown={(e) => inFlight.current && e.preventDefault()} aria-busy={pending || undefined}>
        <AlertDialogHeader>
          <AlertDialogTitle>{title}</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="text-sm text-muted-foreground">{description}</div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        {children}
        {error && (
          <p role="alert" data-slot="confirm-error" className="flex items-start gap-2 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            <CircleAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
            {error}
          </p>
        )}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={pending}>{t("cancel")}</AlertDialogCancel>
          <AlertDialogAction
            className={cn(destructive && buttonVariants({ variant: "destructive" }), "aria-busy:cursor-progress aria-busy:opacity-80")}
            onClick={confirm}
            aria-busy={pending || undefined}
            aria-disabled={pending || undefined}
            data-busy={pending || undefined}
          >
            {pending && <Loader2Icon data-icon="inline-start" className="animate-spin motion-reduce:animate-none" aria-hidden />}
            {confirmLabel}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
