// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original per-host Auto Heal form with stack and container exceptions.

import { X } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { useContainers, useStacks } from "@/data/queries";
import { SettingsProblemError, settingsErrorMessage } from "@/data/settings-error";
import { useAdapter } from "@/data/use-adapter";
import { AUTO_HEAL_LIMITS, WINDOW_PRESETS, autoHealFieldError, type AutoHealInput, type AutoHealSettings } from "@/domain/auto-heal";
import type { Host } from "@/domain/model";
import { HelpLabel, HelpTooltip } from "@/shared/help/help-tooltip";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldContent, FieldDescription, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Slider } from "@/shared/ui/slider";
import { Spinner } from "@/shared/ui/spinner";
import { Switch } from "@/shared/ui/switch";

type ListKey = "excludedStacks" | "excludedContainers";

/** The parent owns the dialog generation, as for notification targets. */
export function AutoHealDialog({ host, settings, onClose, onSaved }: { host: Host; settings: AutoHealSettings; onClose: () => void; onSaved: () => void }) {
  const t = useTranslations("settings.autoHeal");
  const all = useTranslations();
  const adapter = useAdapter();
  const stacks = useStacks();
  const containers = useContainers();
  const [draft, setDraft] = useState<AutoHealInput>(() => ({ enabled: settings.enabled, maxRestarts: settings.maxRestarts, windowMinutes: settings.windowMinutes, excludedStacks: [...settings.excludedStacks], excludedContainers: [...settings.excludedContainers] }));
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<{ message: string; field?: string }>();
  const invalid = autoHealFieldError(draft);
  const windows = [...new Set([...WINDOW_PRESETS, settings.windowMinutes])].sort((a, b) => a - b);
  const known: Record<ListKey, string[]> = {
    excludedStacks: [...new Set((stacks.data ?? []).filter((s) => s.hostId === host.id).map((s) => s.name))].sort(),
    excludedContainers: [...new Set((containers.data ?? []).filter((c) => c.hostId === host.id).map((c) => c.name))].sort(),
  };
  const patch = (value: Partial<AutoHealInput>) => {
    setDraft((old) => ({ ...old, ...value }));
    setError(undefined);
  };
  const fieldError = (key: keyof AutoHealInput) => (invalid === key || error?.field === key ? t("fieldInvalid") : undefined);

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t("dialogTitle", { host: host.name })}</DialogTitle>
          <DialogDescription>{t("scopeHint")}</DialogDescription>
        </DialogHeader>
        <form
          className="flex flex-col gap-5"
          onSubmit={async (event) => {
            event.preventDefault();
            if (invalid) return;
            setBusy(true);
            setError(undefined);
            try {
              await adapter.saveAutoHealSettings(host.id, draft);
              onSaved();
            } catch (cause) {
              const field = cause instanceof SettingsProblemError ? cause.field?.replace(/^\//, "").split("/")[0] : undefined;
              setError({ message: settingsErrorMessage(cause, all), field });
            } finally {
              setBusy(false);
            }
          }}
        >
          <Field orientation="horizontal">
            <Switch id="heal-enabled" checked={draft.enabled} onCheckedChange={(enabled) => patch({ enabled })} />
            <FieldContent>
              <HelpLabel htmlFor="heal-enabled" topic="settingsHeal.enabled">{t("enabled")}</HelpLabel>
              <FieldDescription>{t("enabledHint")}</FieldDescription>
            </FieldContent>
          </Field>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field data-invalid={fieldError("maxRestarts") ? true : undefined}>
              <div className="flex items-center justify-between">
                <HelpLabel htmlFor="heal-max" topic="settingsHeal.maxRestarts">{t("maxRestarts")}</HelpLabel>
                <span className="text-xs text-muted-foreground tabular">{t("maxRestartsValue", { count: draft.maxRestarts })}</span>
              </div>
              <Slider id="heal-max" value={[draft.maxRestarts]} min={AUTO_HEAL_LIMITS.maxRestarts.min} max={AUTO_HEAL_LIMITS.maxRestarts.max} step={1} onValueChange={([v]) => patch({ maxRestarts: v ?? draft.maxRestarts })} aria-label={t("maxRestarts")} />
              {fieldError("maxRestarts") && <p className="text-sm text-destructive">{fieldError("maxRestarts")}</p>}
            </Field>
            <Field data-invalid={fieldError("windowMinutes") ? true : undefined}>
              <HelpLabel htmlFor="heal-window" topic="settingsHeal.window">{t("window")}</HelpLabel>
              <Select value={String(draft.windowMinutes)} onValueChange={(v) => patch({ windowMinutes: Number(v) })}>
                <SelectTrigger id="heal-window" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {windows.map((w) => (
                      <SelectItem key={w} value={String(w)}>{t("windowValue", { minutes: w })}</SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
              {fieldError("windowMinutes") && <p className="text-sm text-destructive">{fieldError("windowMinutes")}</p>}
            </Field>
          </div>
          {(["excludedStacks", "excludedContainers"] as const).map((key) => (
            <Exclusions key={key} id={key} names={draft[key]} known={known[key]} error={fieldError(key)} onChange={(names) => patch({ [key]: names })} />
          ))}
          <p className="text-xs text-muted-foreground">{t("limitNote")}</p>
          {error && <p role="alert" className="text-sm text-destructive">{error.message}</p>}
          <DialogFooter>
            <Button type="submit" disabled={busy || !!invalid}>
              {busy && <Spinner data-icon="inline-start" />}
              {t("save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/** Names from the host, plus free entry for names that do not exist yet. */
function Exclusions({ id, names, known, error, onChange }: { id: ListKey; names: string[]; known: string[]; error?: string; onChange: (names: string[]) => void }) {
  const t = useTranslations("settings.autoHeal");
  const [entry, setEntry] = useState("");
  const add = (name: string) => {
    const value = name.trim();
    if (value && !names.includes(value)) onChange([...names, value]);
  };
  const offered = known.filter((n) => !names.includes(n));
  return (
    <FieldSet className="gap-2" data-invalid={error ? true : undefined}>
      <FieldLegend variant="label" className="mb-1 flex items-center gap-1">
        {t(id)}
        <HelpTooltip topic={id === "excludedStacks" ? "settingsHeal.excluded" : "settingsHeal.excludedContainers"} />
      </FieldLegend>
      {names.length > 0 ? (
        <ul className="flex flex-wrap gap-1.5" aria-label={t(id)}>
          {names.map((n) => (
            <li key={n} className="flex items-center gap-1 rounded-md border py-0.5 pr-0.5 pl-2 font-mono text-xs">
              {n}
              <Button type="button" size="icon" variant="ghost" className="size-5" aria-label={t("removeName", { name: n })} onClick={() => onChange(names.filter((x) => x !== n))}>
                <X />
              </Button>
            </li>
          ))}
        </ul>
      ) : <p className="text-xs text-muted-foreground">{t("noneExcluded")}</p>}
      <div className="flex flex-wrap gap-2">
        {offered.length > 0 && (
          <Select value="" onValueChange={(v) => add(v)}>
            <SelectTrigger className="w-full sm:w-56" aria-label={t(`${id}Pick`)}>
              <SelectValue placeholder={t(`${id}Pick`)} />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                {offered.map((n) => <SelectItem key={n} value={n}>{n}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
        )}
        <div className="flex min-w-0 flex-1 gap-2">
          <Input
            aria-label={t(`${id}Entry`)}
            placeholder={t(`${id}Entry`)}
            className="font-mono text-[13px]"
            spellCheck={false}
            value={entry}
            onChange={(e) => setEntry(e.target.value)}
            onKeyDown={(e) => {
              if (e.key !== "Enter") return;
              e.preventDefault();
              add(entry);
              setEntry("");
            }}
          />
          <Button type="button" variant="outline" disabled={!entry.trim()} onClick={() => { add(entry); setEntry(""); }}>{t("addName")}</Button>
        </div>
      </div>
      {error && <p className="text-sm text-destructive">{error}</p>}
    </FieldSet>
  );
}
