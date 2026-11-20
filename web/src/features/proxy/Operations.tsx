// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { CertificateObservation } from "./CertificateObservation";
import { proxyOutcome } from "@/data/http/proxy-decoder";
import { Button } from "@/shared/ui/button";
import { ToneBadge } from "@/shared/components/tone";
import { ConfirmCheck, Diagnostic, ProxyDialog, Reason } from "./controls";
import type { FileState, Operation, Release } from "./types";

function TargetState({ title, state }: { title: string; state?: FileState }) {
  const t = useTranslations("proxyContract.ui");
  return (
    <div className="min-w-0 text-sm">
      <strong>{title}</strong>
      <p className="break-all">{state ? t(state.present ? "present" : "absent", { hash: state.hash }) : "—"}</p>
    </div>
  );
}
function ReleaseDialog({
  operation,
  busy,
  release,
  close,
}: {
  operation: Operation;
  busy: boolean;
  release: (input: Release) => Promise<void>;
  close: () => void;
}) {
  const t = useTranslations("proxyContract.ui");
  const [confirmed, setConfirmed] = useState(false);
  return (
    <ProxyDialog title={t("release")} description={t("releaseHint")} onClose={close} busy={busy}>
      <p className="break-all">{operation.target}</p>
      <TargetState title={t("current")} state={operation.current} />
      <ConfirmCheck checked={confirmed} onChange={setConfirmed}>
        {t("releaseAck")}
      </ConfirmCheck>
      <Button
        disabled={!confirmed || !operation.current || busy}
        onClick={() => {
          if (confirmed && operation.current && !busy) void release({ confirmed: { ...operation.current } });
        }}
      >
        {t("release")}
      </Button>
    </ProxyDialog>
  );
}
export function Operations({
  rows,
  busy,
  release,
}: {
  rows: Operation[];
  busy: boolean;
  release: (id: string, input: Release) => Promise<void>;
}) {
  const t = useTranslations(),
    ui = useTranslations("proxyContract.ui");
  const [selected, setSelected] = useState<Operation>();
  return (
    <section className="grid min-w-0 gap-3" aria-label={ui("operations")}>
      <h2 className="text-lg font-semibold">{ui("operations")}</h2>
      {rows.map(row => {
        const outcome = proxyOutcome(row);
        return (
          <article key={row.id} className="grid min-w-0 gap-3 rounded-xl border bg-card p-4" data-operation={row.id}>
            <div className="flex flex-wrap items-center gap-2">
              <ToneBadge
                tone={({ success: "ok", warning: "warn", danger: "err", neutral: "info" } as const)[outcome.tone]}
              >
                {t(outcome.labelKey)}
              </ToneBadge>
              {row.state === "pending" && <span>{t(`proxyContract.phases.${row.phase}`)}</span>}
              {row.evidence && <span className="text-sm">{ui(`evidence.${row.evidence}`)}</span>}
            </div>
            <p className="break-all font-mono text-xs">{row.target}</p>
            <Reason code={row.reason} />
            <Diagnostic value={row.diagnostic} />
            {row.probe?.client === "wget" && <p className="text-xs">{ui("certificateUnavailable")}</p>}
            {row.certificate && <CertificateObservation value={row.certificate} />}
            {row.state === "recovery-required" && (
              <>
                <div className="grid min-w-0 gap-3 sm:grid-cols-3">
                  <TargetState title={ui("old")} state={row.old} />
                  <TargetState title={ui("new")} state={row.new} />
                  <TargetState title={ui("current")} state={row.current} />
                </div>
                <p className="text-sm">{ui("releaseHint")}</p>
                <Button variant="outline" disabled={!row.current || busy} onClick={() => setSelected(row)}>
                  {ui("release")}
                </Button>
              </>
            )}
          </article>
        );
      })}
      {selected && (
        <ReleaseDialog
          operation={selected}
          busy={busy}
          close={() => setSelected(undefined)}
          release={async input => {
            await release(selected.id, input);
            setSelected(undefined);
          }}
        />
      )}
    </section>
  );
}
