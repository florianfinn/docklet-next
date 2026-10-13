import { CircleCheck, CircleDot, RotateCcw, Save } from "lucide-react";
import { useRef, useState, type ReactNode } from "react";
import { useBlocker } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { errorCode } from "@/data/api";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { useDemoText } from "@/shared/hooks/use-demo-text";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Spinner } from "@/shared/ui/spinner";

interface SettingsFormProps {
  title: string;
  description?: ReactNode;
  dirty: boolean;
  /** False blocks saving and shows the hint; fields render their own messages. */
  valid?: boolean;
  onSave: () => Promise<void>;
  onDiscard: () => void;
  children: ReactNode;
  note?: ReactNode;
}

/**
 * Frame of one settings section: content, sticky save bar with dirty state,
 * and a confirmation before leaving the route with unsaved changes.
 */
export function SettingsForm({ title, description, dirty, valid = true, onSave, onDiscard, children, note }: SettingsFormProps) {
  const t = useTranslations("settings.form");
  const te = useTranslations("settings.errors");
  const dt = useDemoText();
  const [busy, setBusy] = useState(false);
  const [tried, setTried] = useState(false);
  const blocker = useBlocker(({ currentLocation, nextLocation }) => dirty && currentLocation.pathname !== nextLocation.pathname);
  // The dialog closes right after "proceed"; that close must not reset the blocker.
  const proceeding = useRef(false);

  const save = async () => {
    setTried(true);
    if (!valid) return;
    setBusy(true);
    try {
      await onSave();
      setTried(false);
      // The save bar's status ("Alles gespeichert") confirms it beside the button; a toast would cover page controls.
    } catch (e) {
      const code = errorCode(e);
      toast.error(code === "last-admin" || code === "target-in-use" || code === "invalid-plan" || code === "invalid-history" ? te(code) : dt(e instanceof Error ? e.message : String(e)));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="max-w-5xl gap-0 overflow-hidden p-0">
      <header className="border-b px-5 py-4">
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        {description && <p className="mt-0.5 text-sm text-muted-foreground">{description}</p>}
      </header>
      <form
        className="flex flex-col"
        onSubmit={(e) => {
          e.preventDefault();
          // Dialog forms are portalled but their submit still bubbles through React.
          if (e.target !== e.currentTarget) return;
          void save();
        }}
      >
        <div className="flex flex-col gap-6 p-5">{children}</div>
        {note && <div className="px-5 pb-4 text-xs text-muted-foreground">{note}</div>}
        <footer className="sticky bottom-0 z-10 flex flex-wrap items-center gap-2 border-t bg-card/95 px-5 py-3 backdrop-blur sm:pr-20">
          <span role="status" className={cn("mr-auto inline-flex items-center gap-1.5 text-xs", dirty ? "text-warning" : "text-muted-foreground")}>
            {dirty ? <CircleDot className="size-3.5" aria-hidden /> : <CircleCheck className="size-3.5" aria-hidden />}
            {tried && !valid ? t("invalid") : dirty ? t("dirty") : t("clean")}
          </span>
          <Button type="button" variant="outline" disabled={!dirty || busy} onClick={() => {
            setTried(false);
            onDiscard();
          }}>
            <RotateCcw data-icon="inline-start" />
            {t("discard")}
          </Button>
          <Button type="submit" disabled={!dirty || busy}>
            {busy ? <Spinner data-icon="inline-start" /> : <Save data-icon="inline-start" />}
            {busy ? t("saving") : t("save")}
          </Button>
        </footer>
      </form>
      <ConfirmDialog
        open={blocker.state === "blocked"}
        onOpenChange={(o) => {
          if (!o && !proceeding.current && blocker.state === "blocked") blocker.reset();
          proceeding.current = false;
        }}
        title={t("leaveTitle")}
        description={t("leaveBody", { section: title })}
        confirmLabel={t("leaveConfirm")}
        destructive
        onConfirm={() => {
          if (blocker.state !== "blocked") return;
          proceeding.current = true;
          blocker.proceed();
        }}
      />
    </Card>
  );
}
