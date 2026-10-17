import { Download, FileArchive, FileCode2, KeyRound, RefreshCw, ScrollText, Terminal } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslations } from "use-intl";

import { buildInstallArtifacts, installPackageFiles, type AgentInstallPlan } from "@/domain/agent-install";
import { formatBytes } from "@/domain/format";
import { slugify } from "@/domain/validation";
import { CodeBlock } from "@/shared/components/code-block";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { createTar, downloadBlob } from "@/shared/lib/tar";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";

type Way = "script" | "compose" | "package";

/** Installer command, compose command and downloadable package for one agent plan. */
export function InstallArtifacts({ plan, onRegenerateToken }: { plan: AgentInstallPlan; onRegenerateToken?: () => void }) {
  const t = useTranslations("hosts.install");
  const [way, setWay] = useState<Way>("script");
  const a = useMemo(() => buildInstallArtifacts(plan), [plan]);
  const slug = slugify(plan.hostName);
  const icons = [<FileCode2 />, <KeyRound />, <Terminal />, <ScrollText />];
  const files = installPackageFiles(a).map((file, i) => ({ ...file, icon: icons[i] }));
  const tarName = `docklet-agent-${slug}.tar`;

  return (
    <div className="flex flex-col gap-4">
      <Alert className="border-warning/30 bg-warning/8">
        <KeyRound className="text-warning" />
        <AlertDescription className="text-xs">{t("historyWarning")}</AlertDescription>
      </Alert>
      <div className="flex flex-wrap items-center gap-3 rounded-xl border bg-card/40 p-3">
        <div className="min-w-0 flex-1">
          <div className="text-xs font-medium text-muted-foreground">{t("token")}</div>
          <code className="block truncate font-mono text-[13px]" data-testid="agent-token">
            {plan.token}
          </code>
          <div className="text-[11px] text-muted-foreground">{t("tokenHint")}</div>
        </div>
        {onRegenerateToken && (
          <Button variant="outline" size="sm" className="group" onClick={onRegenerateToken}>
            <RefreshCw data-icon="inline-start" className="spin-on-hover" />
            {t("regenerateToken")}
          </Button>
        )}
      </div>
      <SegmentedToggle
        value={way}
        onChange={setWay}
        label={t("wayLabel")}
        options={[
          { value: "script", label: t("tabs.script"), icon: <Terminal /> },
          { value: "compose", label: t("tabs.compose"), icon: <FileCode2 /> },
          { value: "package", label: t("tabs.package"), icon: <FileArchive /> },
        ]}
        className="self-start"
      />
      {way === "script" && (
        <div className="flex flex-col gap-2">
          <p className="text-sm text-muted-foreground">{t("scriptHint")}</p>
          <CodeBlock code={a.scriptCommand} label="sh – install-docklet-agent.sh" copyLabel={t("copyCommand")} primary />
        </div>
      )}
      {way === "compose" && (
        <div className="flex flex-col gap-2">
          <p className="text-sm text-muted-foreground">{t("composeHint")}</p>
          <CodeBlock code={a.composeCommand} label={`sh – ${plan.installDir}`} copyLabel={t("copyCommand")} primary />
          <CodeBlock code={a.compose} label={`${plan.installDir}/compose.yaml`} maxHeight="14rem" />
        </div>
      )}
      {way === "package" && (
        <div className="flex flex-col gap-3">
          <p className="text-sm text-muted-foreground">{t("packageHint")}</p>
          <Button className="self-start" onClick={() => downloadBlob(tarName, createTar(files.map((f) => ({ name: `docklet-agent/${f.name}`, content: f.content, mode: f.mode }))), "application/x-tar")}>
            <Download data-icon="inline-start" />
            {t("downloadPackage")}
          </Button>
          <ul className="grid gap-2 sm:grid-cols-2">
            {files.map((f) => (
              <li key={f.name} className="flex items-center gap-3 rounded-xl border bg-card/40 p-2.5">
                <span className="text-muted-foreground [&_svg]:size-4">{f.icon}</span>
                <span className="min-w-0 flex-1">
                  <span className="block font-mono text-sm">{f.name}</span>
                  <span className="text-xs text-muted-foreground">{formatBytes(new TextEncoder().encode(f.content).length)}</span>
                </span>
                <Button size="icon" variant="ghost" className="size-8" aria-label={t("downloadFile", { name: f.name })} onClick={() => downloadBlob(f.name === ".env" ? `${slug}.env` : f.name, f.content, "text/plain")}>
                  <Download />
                </Button>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
