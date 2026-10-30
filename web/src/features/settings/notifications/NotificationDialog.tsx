// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original per-channel target form; secrets stay in component state only.

import { CancelledError } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { SettingsProblemError, settingsErrorMessage } from "@/data/settings-error";
import { DEFAULT_EVENTS, NOTIFICATION_EVENTS, NOTIFICATION_PROVIDERS, type NotificationConfig, type NotificationEvent, type NotificationProvider, type NotificationTarget, type NotificationTargetInput } from "@/domain/notifications";
import { TextField } from "@/shared/components/form-field";
import { HelpLegend } from "@/shared/help/help-tooltip";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldContent, FieldLabel, FieldSet } from "@/shared/ui/field";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Spinner } from "@/shared/ui/spinner";
import { Switch } from "@/shared/ui/switch";
import { Textarea } from "@/shared/ui/textarea";

import { DEFAULTS, FIELDS, TARGET_FIELD, fromText, toText, type FieldSpec } from "./fields";
import { ProviderIcon } from "./ProviderIcon";

/**
 * Edits one channel. The parent owns the dialog generation and the per-provider write:
 * `onSaved` and `onClose` carry no state of their own, so a late result cannot reach a
 * replaced dialog, and `save` keeps the provider busy beyond this dialog's lifetime.
 */
export function NotificationDialog({ target, available, save, onClose, onSaved }: {
  target?: NotificationTarget;
  /** Providers without a stored target or pending write, offered when adding. */
  available: readonly NotificationProvider[];
  save: (input: NotificationTargetInput) => Promise<unknown>;
  onClose: () => void;
  onSaved: () => void;
}) {
  const t = useTranslations("settings.notifications");
  const all = useTranslations();
  const [provider, setProvider] = useState<NotificationProvider | undefined>(target?.provider);
  const [config, setConfig] = useState<NotificationConfig>(() => (target ? structuredClone(target.config) : {}));
  const [texts, setTexts] = useState<Record<string, string>>({});
  const [events, setEvents] = useState<readonly NotificationEvent[]>(target?.events ?? DEFAULT_EVENTS);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<{ message: string; field?: string }>();

  // Display text, values and errors belong to one provider choice; a new choice starts clean.
  const choose = (p: NotificationProvider | undefined) => {
    setProvider(p);
    setConfig(p ? structuredClone(DEFAULTS[p]) : {});
    setTexts({});
    setError(undefined);
  };
  const stored = new Set(target?.credentialsSet ?? []);
  const targetField = provider && TARGET_FIELD[provider];
  const addressChanged = !!target && !!targetField && config[targetField] !== target.config[targetField];
  const secretsMissing = addressChanged && [...stored].some((key) => !config[key]);

  const value = (spec: FieldSpec) => texts[spec.key] ?? toText(spec.kind, config[spec.key]);
  const set = (spec: FieldSpec, text: string) => {
    setTexts((old) => ({ ...old, [spec.key]: text }));
    setConfig((old) => ({ ...old, [spec.key]: fromText(spec.kind, text, provider!, spec.key) }));
    if (error?.field === spec.key) setError(undefined);
  };
  const fieldError = (key: string) => (error?.field === key ? t("fieldInvalid") : undefined);

  const field = (spec: FieldSpec) => {
    const id = `nt-${spec.key}`;
    const label = t(`fields.${spec.key}` as never);
    if (spec.kind === "switch")
      return (
        <Field key={spec.key} orientation="horizontal">
          <Switch id={id} checked={config[spec.key] === true} onCheckedChange={(v) => setConfig((old) => ({ ...old, [spec.key]: v }))} />
          <FieldContent>
            <FieldLabel htmlFor={id}>{label}</FieldLabel>
          </FieldContent>
        </Field>
      );
    if (spec.kind === "select")
      return (
        <Field key={spec.key} data-invalid={fieldError(spec.key) ? true : undefined}>
          <FieldLabel htmlFor={id}>{label}</FieldLabel>
          <Select value={value(spec)} onValueChange={(v) => set(spec, v)}>
            <SelectTrigger id={id} className="w-full" aria-invalid={fieldError(spec.key) ? true : undefined}>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {spec.options!.map((o) => (
                <SelectItem key={o} value={o}>
                  {spec.key === "tlsMode" ? t(`tlsModes.${o}` as never) : o}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {fieldError(spec.key) && <p className="text-sm text-destructive">{fieldError(spec.key)}</p>}
        </Field>
      );
    if (spec.kind === "list" || spec.kind === "headers")
      return (
        <Field key={spec.key} data-invalid={fieldError(spec.key) ? true : undefined}>
          <FieldLabel htmlFor={id}>{label}</FieldLabel>
          <Textarea id={id} rows={3} className="font-mono text-[13px]" spellCheck={false} value={value(spec)} onChange={(e) => set(spec, e.target.value)} aria-invalid={fieldError(spec.key) ? true : undefined} aria-describedby={`${id}-hint`} />
          <p id={`${id}-hint`} className="text-xs text-muted-foreground">{spec.kind === "list" ? t("listHint") : t("headersHint")}</p>
          {fieldError(spec.key) && <p className="text-sm text-destructive">{fieldError(spec.key)}</p>}
        </Field>
      );
    const secret = spec.kind === "secret";
    return (
      <TextField
        key={spec.key}
        id={id}
        label={label}
        type={secret ? "password" : spec.kind === "number" ? "number" : "text"}
        mono={spec.kind !== "text"}
        value={value(spec)}
        onChange={(v) => set(spec, v)}
        autoComplete={secret ? "new-password" : undefined}
        help={secret ? "settingsNotifications.secret" : undefined}
        description={secret && stored.has(spec.key) ? t("secretKeep") : spec.kind === "host" ? t("hostHint") : undefined}
        error={fieldError(spec.key)}
      />
    );
  };

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{provider ? (target ? t("dialogEdit", { kind: t(`kinds.${provider}`) }) : t("dialogAdd", { kind: t(`kinds.${provider}`) })) : t("chooseKind")}</DialogTitle>
          <DialogDescription>{provider ? t("dialogHint") : t("chooseHint")}</DialogDescription>
        </DialogHeader>
        {!provider ? (
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-3">
            {NOTIFICATION_PROVIDERS.filter((p) => available.includes(p)).map((p) => (
              <Button key={p} type="button" variant="outline" className="justify-start" onClick={() => choose(p)}>
                <ProviderIcon provider={p} />
                {t(`kinds.${p}`)}
              </Button>
            ))}
          </div>
        ) : (
          <form
            className="flex flex-col gap-4"
            onSubmit={async (event) => {
              event.preventDefault();
              setBusy(true);
              setError(undefined);
              try {
                await save({ provider, enabled: target?.enabled ?? true, config, events });
                onSaved();
              } catch (cause) {
                if (cause instanceof CancelledError) return;
                const pointer = cause instanceof SettingsProblemError ? cause.field?.match(/^\/config\/([^/]+)/)?.[1] : undefined;
                setError({ message: settingsErrorMessage(cause, all), field: pointer });
              } finally {
                setBusy(false);
              }
            }}
          >
            {provider === "generic" && <p className="rounded-lg border bg-card/40 p-3 text-xs text-muted-foreground">{t("genericVisible")}</p>}
            {FIELDS[provider].map(field)}
            {secretsMissing && <p className="text-xs text-warning">{t("addressNeedsSecret")}</p>}
            <FieldSet className="gap-2">
              <HelpLegend topic="settingsNotifications.events" className="mb-1">
                {t("events")}
              </HelpLegend>
              <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
                {NOTIFICATION_EVENTS.map((e) => (
                  <Label key={e} className="flex items-center gap-2 rounded-lg border px-2 py-1.5 text-sm font-normal">
                    <Checkbox checked={events.includes(e)} onCheckedChange={(v) => setEvents(v === true ? [...events, e] : events.filter((x) => x !== e))} />
                    {t(`eventNames.${e}`)}
                  </Label>
                ))}
              </div>
              {fieldError("events") && <p className="text-sm text-destructive">{fieldError("events")}</p>}
            </FieldSet>
            {error && <p role="alert" className="text-sm text-destructive">{error.message}</p>}
            <DialogFooter className="gap-2 sm:justify-between">
              {!target ? (
                <Button type="button" variant="ghost" disabled={busy} onClick={() => choose(undefined)}>
                  <ArrowLeft data-icon="inline-start" />
                  {t("back")}
                </Button>
              ) : <span />}
              <Button type="submit" disabled={busy}>
                {busy && <Spinner data-icon="inline-start" />}
                {t("save")}
              </Button>
            </DialogFooter>
          </form>
        )}
      </DialogContent>
    </Dialog>
  );
}
