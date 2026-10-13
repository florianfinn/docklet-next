import { Lock, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { formatArgs, healthPresets, httpPort, presetCommand, sameTest, type HealthPreset } from "@/domain/compose/health-presets";
import type { ServiceModel } from "@/domain/compose/model";
import { ops, type Mutator } from "@/domain/compose/ops";
import type { Issue } from "@/domain/compose/validate";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { Badge } from "@/shared/ui/badge";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { HelpLabel, HelpTooltip } from "@/shared/help/help-tooltip";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Switch } from "@/shared/ui/switch";

import { useEditorText } from "./editor-text";
import { CommitInput } from "./fields";
import { HealthTestField } from "./HealthTestField";

export interface SectionProps {
  service: ServiceModel;
  issues: Issue[];
  edit: (fn: Mutator) => void;
  onRename?: (to: string) => void;
}

const RESTART = ["no", "always", "on-failure", "unless-stopped"];

function RowIssue({ issues, index }: { issues: Issue[]; index: number }) {
  const text = useEditorText();
  const hit = issues.find((i) => i.ref === String(index + 1));
  return hit ? <p className={hit.level === "error" ? "col-span-full text-xs text-destructive" : "col-span-full text-xs text-warning"}>{text.issue(hit)}</p> : null;
}

export function GeneralSection({ service, issues, edit, onRename }: SectionProps) {
  const t = useTranslations("editor.services");
  const text = useEditorText();
  const imageIssue = issues.find((i) => i.field === "image");
  return (
    <FieldGroup className="gap-5">
      <div className="grid gap-5 md:grid-cols-2">
        <Field>
          <HelpLabel htmlFor={`svc-key-${service.key}`} topic="service.key">
            {t("key")}
          </HelpLabel>
          <CommitInput id={`svc-key-${service.key}`} className="font-mono" value={service.key} onCommit={(v) => onRename?.(v)} />
          <FieldDescription>{t("keyHint")}</FieldDescription>
        </Field>
        <Field data-invalid={imageIssue ? true : undefined}>
          <HelpLabel htmlFor={`svc-image-${service.key}`} topic="service.image">
            {t("image")} <span className="text-destructive">*</span>
          </HelpLabel>
          <Input
            id={`svc-image-${service.key}`}
            className="font-mono"
            aria-invalid={imageIssue ? true : undefined}
            placeholder={t("imagePlaceholder")}
            value={service.image}
            onChange={(e) => edit(ops.setField(service.key, "image", e.target.value))}
          />
          {imageIssue ? <FieldError>{text.issue(imageIssue)}</FieldError> : <FieldDescription>{t("imageHint")}</FieldDescription>}
        </Field>
        <Field>
          <HelpLabel htmlFor={`svc-cn-${service.key}`} topic="service.containerName">
            {t("containerName")}
          </HelpLabel>
          <Input id={`svc-cn-${service.key}`} className="font-mono" value={service.containerName} onChange={(e) => edit(ops.setField(service.key, "container_name", e.target.value))} />
          <FieldDescription>{t("containerNameHint")}</FieldDescription>
        </Field>
        <Field>
          <HelpLabel htmlFor={`svc-restart-${service.key}`} topic="service.restart">
            {t("restart")}
          </HelpLabel>
          <Select value={service.restart || "__none"} onValueChange={(v) => edit(ops.setField(service.key, "restart", v === "__none" ? "" : v))}>
            <SelectTrigger id={`svc-restart-${service.key}`} className="w-full font-mono">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectGroup>
                <SelectItem value="__none">{t("restartNone")}</SelectItem>
                {RESTART.map((r) => (
                  <SelectItem key={r} value={r} className="font-mono">
                    {r}
                  </SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
        </Field>
      </div>
      <Field>
        <HelpLabel htmlFor={`svc-cmd-${service.key}`} topic="service.command">
          {t("command")}
        </HelpLabel>
        <Input id={`svc-cmd-${service.key}`} className="font-mono" value={service.command} onChange={(e) => edit(ops.setCommand(service.key, e.target.value))} />
        <FieldDescription>{t("commandHint")}</FieldDescription>
      </Field>
    </FieldGroup>
  );
}

export function PortsSection({ service, issues, edit }: SectionProps) {
  const t = useTranslations("editor.services.ports");
  return (
    <div className="flex flex-col gap-2">
      {service.ports.length === 0 && <p className="text-sm text-muted-foreground">{t("empty")}</p>}
      {service.ports.map((p) =>
        p.editable ? (
          <div key={p.index} className="grid grid-cols-[1fr_1fr_auto] items-end gap-2">
            <Field>
              <FieldLabel htmlFor={`p-h-${service.key}-${p.index}`} className="text-xs">
                {t("host")}
              </FieldLabel>
              <Input id={`p-h-${service.key}-${p.index}`} className="font-mono" value={p.host} placeholder="8080" onChange={(e) => edit(ops.setPort(service.key, p.index, e.target.value, p.container))} />
            </Field>
            <Field>
              <FieldLabel htmlFor={`p-c-${service.key}-${p.index}`} className="text-xs">
                {t("container")}
              </FieldLabel>
              <Input id={`p-c-${service.key}-${p.index}`} className="font-mono" value={p.container} placeholder="80" onChange={(e) => edit(ops.setPort(service.key, p.index, p.host, e.target.value))} />
            </Field>
            <Button variant="ghost" size="icon" onClick={() => edit(ops.removePort(service.key, p.index))} aria-label={t("remove", { index: p.index + 1 })}>
              <Trash2 />
            </Button>
            <RowIssue issues={issues} index={p.index} />
          </div>
        ) : (
          <p key={p.index} className="flex items-center gap-2 rounded-lg border border-dashed p-2 font-mono text-xs text-muted-foreground">
            <Lock className="size-3.5" aria-hidden /> {p.raw} · {t("readOnly")}
          </p>
        ),
      )}
      <div className="flex items-center gap-1 self-start">
        <Button variant="outline" size="sm" className="self-start" onClick={() => edit(ops.addPort(service.key))}>
          <Plus data-icon="inline-start" /> {t("add")}
        </Button>
        <HelpTooltip topic="service.ports" />
      </div>
    </div>
  );
}

export function MountsSection({ service, issues, edit }: SectionProps) {
  const t = useTranslations("editor.services.mounts");
  return (
    <div className="flex flex-col gap-2">
      {service.mounts.length === 0 && <p className="text-sm text-muted-foreground">{t("empty")}</p>}
      {service.mounts.map((m) =>
        m.editable ? (
          <div key={m.index} className="grid grid-cols-[1fr_1fr_auto_auto] items-end gap-2">
            <Field>
              <FieldLabel htmlFor={`m-s-${service.key}-${m.index}`} className="text-xs">
                {t("source")}
              </FieldLabel>
              <Input id={`m-s-${service.key}-${m.index}`} className="font-mono" value={m.source} onChange={(e) => edit(ops.setMount(service.key, m.index, e.target.value, m.target, m.readOnly))} />
            </Field>
            <Field>
              <FieldLabel htmlFor={`m-t-${service.key}-${m.index}`} className="text-xs">
                {t("target")}
              </FieldLabel>
              <Input id={`m-t-${service.key}-${m.index}`} className="font-mono" value={m.target} onChange={(e) => edit(ops.setMount(service.key, m.index, m.source, e.target.value, m.readOnly))} />
            </Field>
            <div className="flex h-9 items-center gap-1.5">
              <Checkbox id={`m-ro-${service.key}-${m.index}`} checked={m.readOnly} disabled={!m.source} onCheckedChange={(v) => edit(ops.setMount(service.key, m.index, m.source, m.target, v === true))} />
              <Label htmlFor={`m-ro-${service.key}-${m.index}`} className="text-xs">
                {t("readOnly")}
              </Label>
            </div>
            <Button variant="ghost" size="icon" onClick={() => edit(ops.removeMount(service.key, m.index))} aria-label={t("remove", { index: m.index + 1 })}>
              <Trash2 />
            </Button>
            <RowIssue issues={issues} index={m.index} />
          </div>
        ) : (
          <p key={m.index} className="flex items-center gap-2 rounded-lg border border-dashed p-2 font-mono text-xs text-muted-foreground">
            <Lock className="size-3.5" aria-hidden /> {m.raw} · {t("longSyntax")}
          </p>
        ),
      )}
      <p className="text-xs text-muted-foreground">{t("hint")}</p>
      <div className="flex items-center gap-1 self-start">
        <Button variant="outline" size="sm" className="self-start" onClick={() => edit(ops.addMount(service.key))}>
          <Plus data-icon="inline-start" /> {t("add")}
        </Button>
        <HelpTooltip topic="service.volumes" />
      </div>
    </div>
  );
}

export function EnvSection({ service, issues, edit }: SectionProps) {
  const t = useTranslations("editor.services.env");
  const text = useEditorText();
  return (
    <div className="flex flex-col gap-2">
      {service.env.length === 0 && <p className="text-sm text-muted-foreground">{t("empty")}</p>}
      {service.envStyle !== "none" && <Badge variant="outline" className="self-start text-muted-foreground">{t("style", { style: t(service.envStyle) })}</Badge>}
      {service.env.map((e) => {
        const hit = issues.find((i) => i.field === "env" && (i.ref === String(e.index + 1) || i.ref === e.key));
        return (
          <div key={`${e.index}-${e.key}`} className="grid grid-cols-[1fr_1.4fr_auto] items-end gap-2">
            <Field>
              <FieldLabel htmlFor={`e-k-${service.key}-${e.index}`} className="text-xs">
                {t("key")}
              </FieldLabel>
              <CommitInput id={`e-k-${service.key}-${e.index}`} className="font-mono" value={e.key} onCommit={(v) => edit(ops.setEnv(service.key, e.index, v, e.value))} />
            </Field>
            <Field>
              <FieldLabel htmlFor={`e-v-${service.key}-${e.index}`} className="text-xs">
                {t("value")}
              </FieldLabel>
              <Input id={`e-v-${service.key}-${e.index}`} className="font-mono" value={e.value} onChange={(ev) => edit(ops.setEnv(service.key, e.index, e.key, ev.target.value))} />
            </Field>
            <Button variant="ghost" size="icon" onClick={() => edit(ops.removeEnv(service.key, e.index))} aria-label={t("remove", { index: e.index + 1 })}>
              <Trash2 />
            </Button>
            {hit && <p className={hit.level === "error" ? "col-span-full text-xs text-destructive" : "col-span-full text-xs text-warning"}>{text.issue(hit)}</p>}
          </div>
        );
      })}
      <div className="flex items-center gap-1 self-start">
        <Button variant="outline" size="sm" className="self-start" onClick={() => edit(ops.addEnv(service.key))}>
          <Plus data-icon="inline-start" /> {t("add")}
        </Button>
        <HelpTooltip topic="service.env" />
      </div>
    </div>
  );
}

export function HealthSection({ service, issues, edit }: SectionProps) {
  const t = useTranslations("editor.services.health");
  const text = useEditorText();
  const [pending, setPending] = useState<HealthPreset | null>(null);
  const testIssue = issues.find((i) => i.code === "health-test-missing");
  const intervalIssue = issues.find((i) => i.code === "health-interval-format");
  const retriesIssue = issues.find((i) => i.code === "health-retries-format");
  const h = service.health;
  const presets = healthPresets(service);
  const port = httpPort(service);
  const id = (part: string) => `h-${part}-${service.key}`;
  const choose = (p: HealthPreset) => {
    // A hand-written test is only replaced after confirmation.
    const written = h.form === "CMD" ? h.args.length > 0 : !!h.test.trim();
    if (h.enabled && written && !sameTest(h, p)) setPending(p);
    else edit(ops.applyHealthPreset(service.key, p));
  };
  return (
    <FieldGroup className="gap-4">
      <Field orientation="horizontal">
        <Switch id={id("on")} checked={h.enabled} onCheckedChange={(v) => edit(ops.setHealthEnabled(service.key, v))} />
        <HelpLabel htmlFor={id("on")} topic="service.health">
          {t("enabled")}
        </HelpLabel>
      </Field>
      {h.enabled && (
        <div className="grid gap-4 md:grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)]">
          <HealthTestField serviceKey={service.key} health={h} inputId={id("t")} issue={testIssue && text.issue(testIssue)} edit={edit} />
          <Field data-invalid={intervalIssue ? true : undefined}>
            <FieldLabel htmlFor={id("i")}>{t("interval")}</FieldLabel>
            <Input id={id("i")} className="font-mono" placeholder="30s" aria-invalid={intervalIssue ? true : undefined} value={h.interval} onChange={(e) => edit(ops.setHealthField(service.key, "interval", e.target.value))} />
            {intervalIssue ? <FieldError>{text.issue(intervalIssue)}</FieldError> : <FieldDescription>{t("intervalHint")}</FieldDescription>}
          </Field>
          <Field data-invalid={retriesIssue ? true : undefined}>
            <FieldLabel htmlFor={id("r")}>{t("retries")}</FieldLabel>
            <Input id={id("r")} className="font-mono" inputMode="numeric" placeholder="3" aria-invalid={retriesIssue ? true : undefined} value={h.retries} onChange={(e) => edit(ops.setHealthField(service.key, "retries", e.target.value))} />
            {retriesIssue ? <FieldError>{text.issue(retriesIssue)}</FieldError> : <FieldDescription>{t("retriesHint")}</FieldDescription>}
          </Field>
        </div>
      )}
      <section aria-labelledby={id("presets")} className="flex flex-col gap-2" data-slot="health-presets">
        <div className="flex items-center gap-1">
          <h4 id={id("presets")} className="text-sm font-medium">
            {t("presetsTitle")}
          </h4>
          <HelpTooltip topic="service.healthPreset" />
        </div>
        <p className="text-xs text-muted-foreground">{presets[0]?.matched ? t("presetsMatched", { image: service.image }) : service.image.trim() ? t("presetsGeneric") : t("presetsNoImage")}</p>
        <div className="grid gap-2 lg:grid-cols-2">
          {presets.map((p) => {
            const name = t(`preset.${p.id}`);
            return (
              <div key={p.id} data-glow="row" data-preset={p.id} className="flex min-w-0 flex-col gap-1.5 rounded-xl border bg-card/50 p-3">
                <div className="flex items-start justify-between gap-2">
                  <span className="text-sm font-medium">{name}</span>
                  <Button type="button" size="sm" variant="outline" className="h-7 shrink-0 text-xs" onClick={() => choose(p)} aria-label={t("applyLabel", { name })}>
                    {t("apply")}
                  </Button>
                </div>
                <code className="rounded-md bg-code px-2 py-1 font-mono text-xs break-all">
                  [{p.form}] {presetCommand(p)}
                </code>
                <p className="text-xs text-muted-foreground">{t("requires", { tools: p.requires.join(", ") })}</p>
                {!p.matched && <p className="text-xs text-warning">{port.known ? t("portKnown", { port: port.port }) : t("portAssumed")}</p>}
                <p className="text-[11px] text-muted-foreground">{t("timing", { interval: p.interval, timeout: p.timeout, retries: p.retries, start: p.startPeriod ?? "none" })}</p>
              </div>
            );
          })}
        </div>
      </section>
      <ConfirmDialog
        open={!!pending}
        onOpenChange={(o) => !o && setPending(null)}
        title={t("replaceTitle")}
        description={
          <div className="flex flex-col gap-2">
            <p>{t("replaceBody")}</p>
            <p>
              <span className="font-medium text-foreground">{t("replaceCurrent")}:</span> <code className="font-mono text-xs break-all">{h.form === "CMD" ? formatArgs(h.args) : h.test}</code>
            </p>
            {pending && (
              <p>
                <span className="font-medium text-foreground">{t("replaceNext")}:</span> <code className="font-mono text-xs break-all">{presetCommand(pending)}</code>
              </p>
            )}
          </div>
        }
        confirmLabel={t("replaceConfirm")}
        onConfirm={() => {
          if (pending) edit(ops.applyHealthPreset(service.key, pending));
        }}
      />
    </FieldGroup>
  );
}

export function NetworksSection({ service, edit }: SectionProps) {
  const t = useTranslations("editor.services.networks");
  if (!service.networksEditable) {
    return (
      <p className="flex items-center gap-2 rounded-lg border border-dashed p-3 text-xs text-muted-foreground">
        <Lock className="size-3.5" aria-hidden /> {service.networks.join(", ")} · {t("readOnly")}
      </p>
    );
  }
  return (
    <Field>
      <HelpLabel htmlFor={`n-${service.key}`} topic="service.networks">
        {t("label")}
      </HelpLabel>
      <CommitInput id={`n-${service.key}`} className="font-mono" value={service.networks.join(", ")} placeholder="web, internal" onCommit={(v) => edit(ops.setNetworks(service.key, v.split(",")))} />
      <FieldDescription>{t("hint")}</FieldDescription>
    </Field>
  );
}

export function ExtrasSection({ service, onOpenRaw }: SectionProps & { onOpenRaw: () => void }) {
  const t = useTranslations("editor.services.extras");
  return (
    <div className="flex flex-col gap-3">
      <p className="text-sm text-muted-foreground">{t("hint")}</p>
      {service.extras.length ? (
        <div className="flex flex-wrap gap-1.5">
          {service.extras.map((x) => (
            <Badge key={x} variant="outline" className="gap-1 font-mono">
              <Lock className="size-3" aria-hidden />
              {x}
            </Badge>
          ))}
        </div>
      ) : (
        <p className="text-xs text-muted-foreground">{t("none")}</p>
      )}
      <Button variant="outline" size="sm" className="self-start" onClick={onOpenRaw}>
        {t("open")}
      </Button>
    </div>
  );
}
