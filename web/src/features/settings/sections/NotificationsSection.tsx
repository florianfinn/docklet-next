import { Pencil, Plus, Send, Trash2 } from "lucide-react";
import { CancelledError } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useNotificationTargets } from "@/data/notification-queries";
import { useConnectedSettings } from "@/data/queries";
import { settingsErrorMessage } from "@/data/settings-error";
import { useAdapter } from "@/data/use-adapter";
import { isProvider, NOTIFICATION_PROVIDERS, type NotificationProvider, type NotificationTarget, type NotificationTargetInput, type NotificationTargetRow } from "@/domain/notifications";
import { ActionPresentationPointer } from "@/features/activity/ActionPresentationSetting";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { QueryError } from "@/shared/components/page";
import { ToneBadge } from "@/shared/components/tone";
import { HelpTarget } from "@/shared/help/help-tooltip";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Spinner } from "@/shared/ui/spinner";
import { Switch } from "@/shared/ui/switch";

import { summary } from "../notifications/fields";
import { NotificationDialog } from "../notifications/NotificationDialog";
import { ProviderIcon } from "../notifications/ProviderIcon";

export function NotificationsSection() {
  const presentation = useConnectedSettings();
  return (
    <div className="flex flex-col gap-5">
      <CapabilityGate operation="listNotificationTargets">
        <Targets />
      </CapabilityGate>
      {presentation.data && <ActionPresentationPointer value={presentation.data.activity.actionPresentation} />}
    </div>
  );
}

/** A dialog instance; results of an earlier generation never close or change a later one. */
interface DialogState {
  generation: number;
  target?: NotificationTarget;
}

function Targets() {
  const t = useTranslations("settings.notifications");
  const ts = useTranslations("settings");
  const all = useTranslations();
  const adapter = useAdapter();
  const targets = useNotificationTargets();
  const generations = useRef(0);
  const [dialog, setDialog] = useState<DialogState>();
  const [remove, setRemove] = useState<NotificationTargetRow>();
  const [pending, setPending] = useState<ReadonlySet<string>>(new Set());
  const [error, setError] = useState<string>();
  const rows = targets.data ?? [];
  // Test feedback is owned by the mounted section and the target identity it was started for.
  const live = useRef({ mounted: true, rows });
  useEffect(() => {
    const state = live.current;
    state.mounted = true;
    return () => { state.mounted = false; };
  }, []);
  useEffect(() => { live.current.rows = targets.data ?? []; }, [targets.data]);

  const open = (target?: NotificationTarget) => setDialog({ generation: ++generations.current, target });
  const close = (generation: number) => setDialog((current) => (current?.generation === generation ? undefined : current));
  const busy = (key: string, on: boolean) => setPending((old) => { const next = new Set(old); if (on) next.add(key); else next.delete(key); return next; });
  /** One full-target write per provider until it and the following refresh settle; it outlives dialogs. */
  const write = async <T,>(provider: NotificationProvider, run: () => Promise<T>): Promise<T> => {
    busy(provider, true);
    try {
      return await run();
    } finally {
      await targets.refetch().catch(() => undefined);
      busy(provider, false);
    }
  };

  const used = new Set(rows.map((r) => r.provider));
  const available = NOTIFICATION_PROVIDERS.filter((p) => !used.has(p) && !pending.has(p));

  const toggle = async (target: NotificationTarget, enabled: boolean) => {
    setError(undefined);
    try {
      await write(target.provider, () => adapter.saveNotificationTarget({ provider: target.provider, enabled, config: target.config, events: target.events }));
    } catch (cause) {
      if (!(cause instanceof CancelledError)) setError(settingsErrorMessage(cause, all));
    }
  };
  const test = async (target: NotificationTarget) => {
    const { provider, id } = target;
    busy(`test:${provider}`, true);
    const kind = t(`kinds.${provider}`);
    const current = () => live.current.mounted && live.current.rows.some((r) => r.id === id && r.provider === provider);
    try {
      const result = await adapter.testNotificationTarget(provider);
      if (!current()) return;
      if (result.code === "notification.test-sent") {
        toast.success(`${kind}: ${all("settings.codes.notification.test-sent")}`);
        if (result.warning) toast.warning(`${kind}: ${all("settings.codes.notification.target-disabled")}`);
      } else {
        const reason = result.reason ? all(`settings.codes.${result.reason}` as never) : "";
        toast.error(`${kind}: ${all("settings.codes.notification.test-failed")}${reason ? ` – ${reason}` : ""}`);
      }
    } catch (cause) {
      if (current() && !(cause instanceof CancelledError)) toast.error(settingsErrorMessage(cause, all));
    } finally {
      busy(`test:${provider}`, false);
    }
  };

  return (
    <section className="flex flex-col gap-4 rounded-xl border bg-card p-6" aria-labelledby="notifications-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <h2 id="notifications-title" className="text-lg font-semibold">{ts("sections.notifications")}</h2>
          <p className="text-sm text-muted-foreground">{t("onlyConfigured")}</p>
        </div>
        <Button type="button" variant="outline" disabled={!targets.data || available.length === 0} onClick={() => open()}>
          <Plus data-icon="inline-start" />
          {t("add")}
        </Button>
      </div>
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
      {targets.isError ? (
        <QueryError error={targets.error} onRetry={() => void targets.refetch()} />
      ) : targets.isPending ? (
        <p role="status" className="text-sm text-muted-foreground">{t("loading")}</p>
      ) : rows.length === 0 ? (
        <p className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground">{t("empty")}</p>
      ) : (
        <ul className="flex flex-col gap-2">
          {rows.map((row) => {
            if (row.kind === "unsupported") {
              const label = isProvider(row.provider) ? t(`kinds.${row.provider}`) : row.provider || t("unknownKind");
              return (
                <li key={row.id} className="flex flex-wrap items-center gap-3 rounded-xl border border-dashed p-3">
                  <span className="grid size-9 place-items-center rounded-xl bg-muted text-muted-foreground [&_svg]:size-4"><ProviderIcon provider={row.provider} /></span>
                  <div className="min-w-0 flex-1">
                    <span className="break-all font-medium">{label}</span>
                    <p className="text-xs text-muted-foreground">{t("unsupported")}</p>
                  </div>
                  {isProvider(row.provider) && (
                    <Button type="button" size="icon" variant="ghost" className="size-8 text-destructive hover:text-destructive" aria-label={`${t("remove")}: ${label}`} disabled={pending.has(row.provider)} onClick={() => setRemove(row)}>
                      <Trash2 />
                    </Button>
                  )}
                </li>
              );
            }
            const label = t(`kinds.${row.provider}`);
            const testing = pending.has(`test:${row.provider}`);
            return (
              <li key={row.id} className={cn("flex flex-wrap items-center gap-3 rounded-xl border bg-card/40 p-3", !row.enabled && "opacity-70")}>
                <span className="grid size-9 place-items-center rounded-xl bg-primary/12 text-primary-text [&_svg]:size-4"><ProviderIcon provider={row.provider} /></span>
                <div className="min-w-0 flex-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-medium">{label}</span>
                    {!row.enabled && <ToneBadge tone="gray" className="h-5 px-1.5 text-[10.5px]">{t("disabled")}</ToneBadge>}
                  </div>
                  <div className="truncate font-mono text-[11.5px] text-muted-foreground">{summary(row.provider, row.config)}</div>
                  <div className="mt-1 flex flex-wrap gap-1">
                    {row.events.length === 0 ? (
                      <span className="text-[11px] text-muted-foreground">{t("noEvents")}</span>
                    ) : row.events.map((e) => (
                      <span key={e} className="rounded-md border px-1.5 py-0.5 text-[10.5px] text-muted-foreground">{t(`eventNames.${e}`)}</span>
                    ))}
                  </div>
                </div>
                <div className="flex items-center gap-1.5">
                  <Switch aria-label={`${t("enabled")}: ${label}`} checked={row.enabled} disabled={pending.has(row.provider)} onCheckedChange={(enabled) => void toggle(row, enabled)} />
                  <HelpTarget topic="settingsNotifications.test">
                    <Button type="button" size="sm" variant="ghost" disabled={testing} onClick={() => void test(row)}>
                      {testing ? <Spinner data-icon="inline-start" /> : <Send data-icon="inline-start" />}
                      {testing ? t("testing") : t("test")}
                    </Button>
                  </HelpTarget>
                  <Button type="button" size="icon" variant="ghost" className="size-8" aria-label={`${t("edit")}: ${label}`} disabled={pending.has(row.provider)} onClick={() => open(row)}>
                    <Pencil />
                  </Button>
                  <Button type="button" size="icon" variant="ghost" className="size-8 text-destructive hover:text-destructive" aria-label={`${t("remove")}: ${label}`} disabled={pending.has(row.provider)} onClick={() => setRemove(row)}>
                    <Trash2 />
                  </Button>
                </div>
              </li>
            );
          })}
        </ul>
      )}
      {dialog && (
        <NotificationDialog
          key={dialog.generation}
          target={dialog.target}
          available={available}
          save={(input: NotificationTargetInput) => write(input.provider, () => adapter.saveNotificationTarget(input))}
          onClose={() => close(dialog.generation)}
          onSaved={() => close(dialog.generation)}
        />
      )}
      <ConfirmDialog
        open={!!remove}
        onOpenChange={(o) => !o && setRemove(undefined)}
        title={t("removeTitle")}
        description={t("removeHint")}
        confirmLabel={t("remove")}
        onConfirm={async () => {
          const provider = remove?.provider;
          if (!isProvider(provider)) return;
          try {
            await write(provider, () => adapter.deleteNotificationTarget(provider));
          } catch (cause) {
            if (!(cause instanceof CancelledError)) setError(settingsErrorMessage(cause, all));
            return false;
          }
        }}
      />
    </section>
  );
}
