import { DialogCommandFooter } from "@/shared/components/dialog-command";
import { useDialogCommand, type DialogCommand } from "@/shared/components/use-dialog-command";
import { isHostAvailable } from "@/domain/hosts";
import { useActionRuns } from "@/data/live";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { useCapability } from "@/data/use-capability";
import { Bot, Cable, Check, RotateCw } from "lucide-react";
import { useState, type ReactNode } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useHostError } from "./use-host-error";
import { useAdapter } from "@/data/use-adapter";
import { formatMem, formatPorts, formatRelative, formatTime } from "@/domain/format";
import type { Host } from "@/domain/model";
import { SYSTEM_LABEL, type SystemComponent } from "@/domain/system";
import { versionLabel } from "@/domain/updates";
import { BusyButton } from "@/shared/components/busy-button";
import { TextField } from "@/shared/components/form-field";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Input } from "@/shared/ui/input";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";

import { defaultHostColor } from "@/domain/hosts";
import { HEX, hostDraftError } from "./host-draft";
import { HOST_COLORS } from "./onboarding/onboarding-draft";
import { useHostAction, useHostCommands } from "./use-host-commands";

function Row({ label, children, mono }: { label: string; children: ReactNode; mono?: boolean }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className={cn("min-w-0 [overflow-wrap:anywhere]", mono && "font-mono text-xs")}>{children}</dd>
    </>
  );
}

function AgentPanel({ host, component, feedback }: { host: Host; component: SystemComponent | undefined; feedback: DialogCommand }) {
  const t = useTranslations("hosts.settings");
  const ts = useTranslations("common.status");
  const cmd = useHostCommands();
  const restartCapability = useCapability("restartAgent", { kind: "host", id: host.id });
  const reconnect = useCapability("reconnectHost", { kind: "host", id: host.id });
  const restart = useHostAction(host.id, "agent-restart", async () => await feedback.run(() => cmd.restartAgent(host.id)));
  const check = useHostAction(host.id, "reconnect", async () => await feedback.run(() => cmd.reconnect(host.id)));
  const [confirmRestart, setConfirmRestart] = useState(false);
  const latestRestart = useActionRuns().find(run => run.hostId === host.id && run.kind === "agent-restart");
  const online = isHostAvailable(host);
  return (
    <div className="flex flex-col gap-4" data-slot="agent-container">
      {component ? <>
      <p className="flex items-start gap-2 rounded-xl border border-dashed bg-muted/15 p-3 text-xs text-muted-foreground">
        <Bot className="mt-0.5 size-4 shrink-0" aria-hidden />
        {t("agentIntro", { label: `${SYSTEM_LABEL}=agent` })}
      </p>
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-sm">
        <Row label={t("container")} mono>
          {component.containerName}
        </Row>
        <Row label={t("project")} mono>
          {component.composeProject}
        </Row>
        <Row label={t("image")}>
          {component.image.repository} · <span className="font-mono">{versionLabel(component.image)}</span>
        </Row>
        <Row label={t("state")}>
          <ToneBadge tone={component.state === "running" ? "ok" : "orange"}>{ts(component.state === "running" ? "running" : "unknown")}</ToneBadge>
        </Row>
        <Row label={t("started")}>{component.startedAt ? formatRelative(component.startedAt) : "–"}</Row>
        <Row label={t("restartPolicy")} mono>
          {component.restartPolicy}
        </Row>
        <Row label={t("memLimit")}>{formatMem(component.memLimitMb)}</Row>
        <Row label={t("ports")} mono>
          {formatPorts(component.ports) || "–"}
        </Row>
        <Row label={t("networks")} mono>
          {component.networks.join(", ")}
        </Row>
        <Row label={t("mounts")} mono>
          <ul className="flex flex-col gap-0.5">
            {component.mounts.map((m) => (
              <li key={m}>{m}</li>
            ))}
          </ul>
        </Row>
        <Row label={t("location")} mono>
          {component.installDir}
        </Row>
        <Row label={t("labels")} mono>
          {Object.entries(component.labels)
            .map(([k, v]) => `${k}=${v}`)
            .join(" · ")}
        </Row>
      </dl>
      <section aria-label={t("log")}>
        <h3 className="mb-1.5 text-xs font-semibold text-muted-foreground">{t("log")}</h3>
        <ol className="max-h-40 overflow-auto rounded-lg border bg-code p-2 font-mono text-[11.5px] leading-relaxed">
          {component.recentLog.map((l) => (
            <li key={l.at + l.message} className={cn("grid grid-cols-[4.5rem_3rem_minmax(0,1fr)] gap-2", l.level === "warn" && "text-warning", l.level === "error" && "text-destructive")}>
              <time dateTime={l.at} className="text-muted-foreground">
                {formatTime(l.at, true)}
              </time>
              <span className="text-muted-foreground">{l.level}</span>
              <span className="[overflow-wrap:anywhere]">{l.message}</span>
            </li>
          ))}
        </ol>
      </section>
      </> : <p className="text-sm text-muted-foreground">{t("agentNone")}</p>}
      <div className="flex flex-wrap items-center gap-2">
        {host.id !== "0" && <BusyButton type="button" size="sm" variant="outline" disabled={!online || !restartCapability.available} title={restartCapability.reason} busy={restart.busy} busyLabel={t("restarting")} icon={<RotateCw data-icon="inline-start" />} onClick={() => setConfirmRestart(true)}>
          {t("restart")}
        </BusyButton>}
        <BusyButton type="button" size="sm" variant="ghost" disabled={!reconnect.available} title={reconnect.reason} busy={check.busy} busyLabel={t("checking")} icon={<Cable data-icon="inline-start" />} onClick={() => void check.run()}>
          {t("check")}
        </BusyButton>
        <span className="text-xs text-muted-foreground">{online ? t("restartHint") : t("restartBlocked")}</span>
      </div>
      {latestRestart && <section aria-label={t("restartProgress")}>
        <ol className="flex flex-col gap-1 text-sm">{latestRestart.steps.map(step => <li key={step.key} data-state={step.state}>{step.label}</li>)}</ol>
        {latestRestart.error && <p role="alert" className="text-sm text-destructive">{latestRestart.error}</p>}
      </section>}
      <ConfirmDialog open={confirmRestart} onOpenChange={setConfirmRestart} title={t("restartTitle", { name: host.name })} description={t("restartBody")} confirmLabel={t("restart")} onConfirm={async () => (await restart.run()) ?? false} />
    </div>
  );
}

/**
 * Per-host settings: display name and colour through the typed adapter
 * mutation, plus the host's own agent container. The
 * draft starts from the saved host each time the dialog opens; Cancel drops it.
 */
export function HostSettingsDialog({ host, component, open, onOpenChange }: { host: Host; component: SystemComponent | undefined; open: boolean; onOpenChange: (open: boolean) => void }) {
  const t = useTranslations("hosts.settings");
  const errorText = useHostError();
  const feedback = useDialogCommand(open, host.id, errorText);
  const change = (next: boolean) => feedback.change(next, onOpenChange);
  const adapter = useAdapter();
  const [name, setName] = useState(host.name);
  const update = useCapability("updateHost", { kind: "host", id: host.id });
  const [color, setColor] = useState(host.color);
  const [resetColor, setResetColor] = useState(false);
  const [hex, setHex] = useState(host.color);
  const [tab, setTab] = useState("general");
  const [saving, setSaving] = useState(false);
  const [serverError, setServerError] = useState<string | null>(null);
  const [touched, setTouched] = useState(false);
  const [seen, setSeen] = useState(open);
  if (open !== seen) {
    setSeen(open);
    if (open) {
      setName(host.name);
      setColor(host.color);
      setHex(host.color);
      setResetColor(false);
      setServerError(null);
      setTouched(false);
      setTab("general");
    }
  }
  const error = hostDraftError(name, color);
  const dirty = resetColor || name.trim() !== host.name || color.toLowerCase() !== host.color.toLowerCase();

  const save = async () => {
    if (!update.available) return;
    setTouched(true);
    if (error || !dirty) return;
    setServerError(null);
    setSaving(true);
    try {
      const saved = await adapter.updateHost(host.id, { name: name.trim(), color: resetColor ? null : color });
      toast.success(t("saved", { name: saved.name }));
      change(false);
    } catch (e) {
      setServerError(errorText(e));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogContent {...feedback.bindings} className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-2xl" data-surface="host-settings">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <span className="size-3 rounded-[4px]" style={{ background: HEX.test(color) ? color : host.color }} aria-hidden />
            {t("title", { name: host.name })}
          </DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        <Tabs value={tab} onValueChange={setTab}>
          <TabsList>
            <TabsTrigger value="general">{t("tabs.general")}</TabsTrigger>
            <TabsTrigger value="agent">{t("tabs.agent")}</TabsTrigger>
          </TabsList>
          <TabsContent value="general" className="mt-3">
            <form
              id={`host-settings-${host.id}`}
              className="flex flex-col gap-4"
              onSubmit={(e) => {
                e.preventDefault();
                void save();
              }}
            >
              <TextField
                id={`host-name-${host.id}`}
                label={t("name")}
                value={name}
                onChange={(v) => {
                  setName(v);
                  setServerError(null);
                }}
                description={t("nameHint")}
                error={(touched || name !== host.name) && (error === "nameEmpty" || error === "nameLong") ? t(error) : undefined}
              />
              {serverError && <p role="alert" className="text-sm text-destructive">{serverError}</p>}
              <fieldset className="flex flex-col gap-2">
                <legend className="mb-1.5 text-sm font-medium">{t("color")}</legend>
                <div className="flex flex-wrap items-center gap-3">
                  <div role="radiogroup" aria-label={t("color")} className="flex flex-wrap gap-1.5">
                    {[...new Set([host.color, ...HOST_COLORS])].map((c) => (
                      <button
                        key={c}
                        type="button"
                        role="radio"
                        aria-checked={color.toLowerCase() === c.toLowerCase()}
                        aria-label={t("colorOption", { color: c })}
                        onClick={() => {
                          setResetColor(false);
                          setColor(c);
                          setHex(c);
                        }}
                        className={cn("grid size-7 place-items-center rounded-lg ring-offset-2 ring-offset-background outline-none focus-visible:ring-2 focus-visible:ring-ring", color.toLowerCase() === c.toLowerCase() && "ring-2 ring-foreground/60")}
                        style={{ background: c }}
                      >
                        {color.toLowerCase() === c.toLowerCase() && <Check className="size-3.5 text-white" strokeWidth={3} />}
                      </button>
                    ))}
                  </div>
                  <Input
                    aria-label={t("colorHex")}
                    aria-invalid={!HEX.test(hex) || undefined}
                    value={hex}
                    onChange={(e) => {
                      setResetColor(false);
                      setHex(e.target.value);
                      if (HEX.test(e.target.value)) setColor(e.target.value.toLowerCase());
                    }}
                    className="h-8 w-28 font-mono text-xs"
                  />
                </div>
                <Button type="button" variant="outline" size="sm" className="self-start" onClick={() => { const value = defaultHostColor(host.id); setColor(value); setHex(value); setResetColor(true); }}>{t("resetColor")}</Button>
                {!HEX.test(hex) && <p className="text-xs text-destructive">{t("colorInvalid")}</p>}
              </fieldset>
              <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card/40 p-3 text-sm" aria-label={t("preview")}>
                <span className="text-xs text-muted-foreground">{t("preview")}</span>
                <span className="inline-flex items-center gap-1.5 rounded-md border px-2 py-0.5 text-xs font-medium" style={{ borderColor: `color-mix(in oklch, ${color} 45%, transparent)` }}>
                  <i className="size-2.5 rounded-[3px]" style={{ background: color }} aria-hidden />
                  {name.trim() || host.name}
                </span>
                <span className="h-5 w-24 rounded-md" style={{ background: `color-mix(in oklch, ${color} 18%, transparent)`, borderLeft: `3px solid ${color}` }} aria-hidden />
              </div>
            </form>
          </TabsContent>
          <TabsContent value="agent" className="mt-3">
            <AgentPanel host={host} component={component} feedback={feedback} />
          </TabsContent>
        </Tabs>
        <DialogCommandFooter feedback={feedback} className="items-center">
          {tab === "general" && !dirty && <span className="mr-auto text-xs text-muted-foreground">{t("unchanged")}</span>}
          <Button type="button" variant="outline" onClick={() => change(false)}>
            {tab === "general" ? t("cancel") : t("close")}
          </Button>
          {tab === "general" && (
            <Button type="submit" form={`host-settings-${host.id}`} disabled={!update.available || !dirty || !!error || !HEX.test(hex) || saving} title={update.reason}>
              {t("save")}
            </Button>
          )}
        </DialogCommandFooter>
      </DialogContent>
    </Dialog>
  );
}
