import { CheckCircle2, Circle, CircleX, Eye, Layers, Loader2, Plus, Server } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useActionRun } from "@/data/live";
import type { AgentInstallPlan } from "@/domain/agent-install";
import type { Host } from "@/domain/model";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { setPreference } from "@/shared/preferences/preferences-store";
import { Alert, AlertDescription, AlertTitle } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Label } from "@/shared/ui/label";

import { AgentReportSection } from "../AgentReportSection";
import { InstallArtifacts } from "../InstallArtifacts";
import { useHostCommands } from "../use-host-commands";
import { onboardingStore, patchDraft, type HostDraft } from "./onboarding-draft";

export function InstallStep({ draft, plan }: { draft: HostDraft; plan: AgentInstallPlan }) {
  const t = useTranslations("hosts.onboarding");
  const rows: [string, string][] = [
    [t("summary.mode"), draft.connection === "direct" ? `${t("directTitle")} · ${draft.address}:${draft.port}` : `${t("edgeTitle")} · ${plan.hubUrl}`],
    [t("summary.project"), plan.projectDir],
    [t("summary.install"), plan.installDir],
    [t("summary.ids"), plan.fileOwner ? `${plan.fileOwner.uid}:${plan.fileOwner.gid}` : t("summary.auto")],
    [t("summary.socket"), plan.socketGid !== undefined ? String(plan.socketGid) : t("summary.auto")],
  ];
  return (
    <div className="flex flex-col gap-4">
      <p className="text-sm text-muted-foreground">{t("installLead", { name: plan.hostName })}</p>
      <dl className="grid gap-x-4 gap-y-1.5 rounded-xl border bg-card/40 p-3.5 text-sm sm:grid-cols-[auto_1fr]">
        {rows.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-muted-foreground">{k}</dt>
            <dd className="font-mono text-xs break-all sm:text-[13px]">{v}</dd>
          </div>
        ))}
      </dl>
      {draft.token ? <InstallArtifacts plan={plan} /> : <div className="h-64 animate-pulse rounded-xl bg-muted/40" />}
    </div>
  );
}

type StageState = "done" | "running" | "pending" | "error";

function Stage({ state, title, detail }: { state: StageState; title: string; detail?: string }) {
  const t = useTranslations("hosts.onboarding.stageState");
  const Icon = state === "done" ? CheckCircle2 : state === "running" ? Loader2 : state === "error" ? CircleX : Circle;
  return (
    <li className="flex gap-3">
      <Icon className={cn("mt-0.5 size-4.5 shrink-0", state === "done" && "text-success", state === "running" && "animate-spin text-primary-text", state === "error" && "text-destructive", state === "pending" && "text-muted-foreground/60")} aria-hidden />
      <div className="min-w-0">
        <div className={cn("text-sm", state === "pending" ? "text-muted-foreground" : "font-medium")}>{title}</div>
        {detail && <div className="text-xs break-words text-muted-foreground">{detail}</div>}
      </div>
      <span className="sr-only">{t(state)}</span>
    </li>
  );
}

export function ConnectStep({ draft, host, hubUrl }: { draft: HostDraft; host: Host | undefined; hubUrl: string }) {
  const t = useTranslations("hosts.onboarding");
  const th = useTranslations("hosts");
  const cmd = useHostCommands();
  const run = useActionRun(draft.runId);
  const connected = host?.status === "online";
  const failed = run?.status === "failed";
  const running = run?.status === "running";

  const start = async () => {
    if (!draft.hostId) return;
    const id = await cmd.connect(draft.hostId, draft.simulateFailure);
    if (id) patchDraft({ runId: id });
  };

  return (
    <div className="flex flex-col gap-5">
      <p className="text-sm text-muted-foreground">{t("connectLead")}</p>
      <ol className="flex flex-col gap-3 rounded-xl border bg-card/40 p-4" aria-label={t("connectTitle")}>
        <Stage state="done" title={t("stageRegistered")} detail={t("stageRegisteredDetail")} />
        <Stage state={run || connected ? "done" : "running"} title={t("stageInstall")} detail={t("stageInstallDetail")} />
        {(run?.steps ?? []).map((s) => (
          <Stage key={s.key} state={s.state === "done" ? "done" : s.state === "running" ? "running" : s.state === "error" ? "error" : "pending"} title={s.label} detail={s.detail} />
        ))}
      </ol>
      {host && <AgentReportSection host={host} />}
      {host?.status === "token-rejected" && <Alert variant="destructive"><AlertDescription>{th("tokenRejected")}</AlertDescription></Alert>}
      {host?.status === "update-required" && <Alert><AlertDescription>{t("updateRequired")}</AlertDescription></Alert>}
      {connected ? (
        <Alert className="animate-pop-in border-success/40 bg-success/8">
          <CheckCircle2 className="text-success" />
          <AlertTitle>{t("connectedTitle", { name: host.name })}</AlertTitle>
          <AlertDescription className="flex flex-col gap-3">
            <span>{t("connectedBody")}</span>
            <div className="flex flex-wrap gap-2">
              <Button asChild size="sm">
                <Link to={`/stacks/new?host=${host.id}`} onClick={() => onboardingStore.set(null)}>
                  <Layers data-icon="inline-start" />
                  {t("newStack")}
                </Link>
              </Button>
              <Button asChild size="sm" variant="outline">
                <Link to="/hosts" onClick={() => onboardingStore.set(null)}>
                  <Server data-icon="inline-start" />
                  {t("toHosts")}
                </Link>
              </Button>
              <Button size="sm" variant="ghost" onClick={() => setPreference("hostSelection", [host.id])}>
                <Eye data-icon="inline-start" />
                {t("showOnly")}
              </Button>
              <Button asChild size="sm" variant="ghost">
                <Link to="/hosts/new" onClick={() => onboardingStore.set(null)}>
                  <Plus data-icon="inline-start" />
                  {t("another")}
                </Link>
              </Button>
            </div>
          </AlertDescription>
        </Alert>
      ) : (
        <div className="flex flex-col gap-3">
          {failed && (
            <Alert variant="destructive" className="animate-pop-in border-destructive/30 bg-destructive/5">
              <CircleX />
              <AlertTitle>{t("failedTitle")}</AlertTitle>
              <AlertDescription>
                <p>{run.error}</p>
                <ul className="mt-1.5 list-disc pl-4 text-xs">
                  <li>{t("hintRunning")}</li>
                  {draft.connection === "direct" ? <li>{t("hintPort", { port: draft.port, address: draft.address })}</li> : <li>{t("hintHub", { hub: hubUrl })}</li>}
                  <li>{t("hintToken")}</li>
                </ul>
              </AlertDescription>
            </Alert>
          )}
          {import.meta.env.DEV && <div className="flex flex-wrap items-center gap-3">
            <Button onClick={() => void start()} disabled={running || !draft.hostId}>
              {running ? <Loader2 data-icon="inline-start" className="animate-spin" /> : null}
              {failed ? t("retry") : t("simulate")}
            </Button>
            <div className="flex items-center gap-2">
              <Checkbox id="host-sim-fail" checked={draft.simulateFailure} onCheckedChange={(v) => patchDraft({ simulateFailure: v === true })} disabled={running} />
              <Label htmlFor="host-sim-fail" className="text-sm font-normal">
                {t("simulateFailure")}
              </Label>
            </div>
            <ToneBadge tone="violet">{t("simulation")}</ToneBadge>
          </div>}
        </div>
      )}
    </div>
  );
}
