// Source: docklet-next/web/src/features/hosts/HostCard.tsx
// Revision: 579ca3a12d93da7c7a6e68686d20425a642423fc
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Share the authenticated agent report between host cards and onboarding.

import { ClipboardCheck, TriangleAlert } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import { formatRelative } from "@/domain/format";
import type { Host } from "@/domain/model";
import { cn } from "@/shared/lib/utils";

function ReportRow({ label, children }: { label: string; children: ReactNode; mono?: boolean }) {
  return <><dt className="text-muted-foreground">{label}</dt><dd className="min-w-0 font-mono text-xs break-words">{children}</dd></>;
}

/** What the installer found locally on the host, as the agent reported it after connecting. */
export function AgentReportSection({ host }: { host: Host }) {
  const t = useTranslations("hosts.card.report");
  const r = host.agentReport;
  const reportTimeKey = host.status === "offline" ? "lastReported" : host.status === "token-rejected" ? "reportedTokenRejected" : host.status === "update-required" ? "reportedUpdateRequired" : host.status === "pending" ? "reportedPending" : r?.simulated ? "reported" : "reportedLive";
  return (
    <section aria-label={t("title")} className="rounded-xl border bg-card/40 p-3">
      <h3 className="mb-1.5 flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
        <ClipboardCheck className="size-3.5" aria-hidden />
        {t("title")}
      </h3>
      {!r ? (
        <p className="text-xs text-muted-foreground">{t("pending")}</p>
      ) : (
        <>
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
            <ReportRow label={t("owner")} mono>
              {t("ownerValue", { name: r.projectDirOwner.name, uid: r.projectDirOwner.uid, gid: r.projectDirOwner.gid, writable: String(r.projectDirWritable) })}
            </ReportRow>
            {r.fileOwner && r.fileOwnerSource ? <ReportRow label={t("fileIds")} mono>
              {t("fileIdsValue", { uid: r.fileOwner.uid, gid: r.fileOwner.gid, source: r.fileOwnerSource })}
            </ReportRow> : null}
            {r.socketGid !== undefined ? <ReportRow label={t("socket")} mono>
              {t("socketValue", { gid: r.socketGid, source: r.socketGidSource ?? "detected" })}
            </ReportRow> : null}
          </dl>
          {r.note !== "ok" && (
            <p className={cn("mt-1.5 flex items-start gap-1.5 text-xs", r.note === "root-owned" ? "text-warning" : "text-muted-foreground")}>
              {r.note === "root-owned" && <TriangleAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />}
              {t(r.note === "root-owned" ? "root" : "unraid")}
            </p>
          )}
          <p className="mt-1.5 text-[11px] text-muted-foreground">{t(reportTimeKey, { time: formatRelative(r.reportedAt) })}</p>
        </>
      )}
    </section>
  );
}

