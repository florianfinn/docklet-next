import type { UseQueryResult } from "@tanstack/react-query";
import { Check, RefreshCw, Radar } from "lucide-react";
import { useTranslations } from "use-intl";

import { hubAddressCandidates, type CandidateReach, type HubAddressCandidate, type HubDiscovery } from "@/domain/hub-discovery";
import type { Tone } from "@/domain/model";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Spinner } from "@/shared/ui/spinner";

import type { HubAddressOrigin } from "./setup-draft";

const REACH_TONE: Record<CandidateReach, Tone> = { lan: "ok", "same-host": "warn", unknown: "gray" };

interface Props {
  discovery: UseQueryResult<HubDiscovery>;
  address: string;
  origin: HubAddressOrigin;
  onPick: (candidate: HubAddressCandidate) => void;
}

/**
 * Address proposals from the hub's own server-side inspection. Each entry says
 * where it comes from and who can reach it; the field stays freely editable.
 */
export function HubAddressDiscovery({ discovery, address, origin, onPick }: Props) {
  const t = useTranslations("setup.discovery");
  const data = discovery.data;
  const candidates = data ? hubAddressCandidates(data) : [];
  const picked = origin?.kind === "discovered" ? candidates.find((c) => c.id === origin.candidateId) : undefined;

  return (
    <section className="flex flex-col gap-3 rounded-xl border bg-card/40 p-4" aria-label={t("title")}>
      <div className="flex items-start gap-2">
        <Radar className="mt-0.5 size-4 shrink-0 text-primary-text" aria-hidden />
        <div className="min-w-0 flex-1">
          <h3 className="text-sm font-semibold">{t("title")}</h3>
          <p className="text-xs text-muted-foreground">{t("lead")}</p>
        </div>
        {data?.simulated && <ToneBadge tone="violet">{t("simulated")}</ToneBadge>}
      </div>
      {discovery.isPending ? (
        <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
          <Spinner /> {t("loading")}
        </p>
      ) : discovery.isError ? (
        <div className="flex flex-wrap items-center gap-2 text-sm" role="alert">
          <span className="text-warning">{t("failed")}</span>
          <Button size="sm" variant="outline" onClick={() => void discovery.refetch()}>
            <RefreshCw data-icon="inline-start" />
            {t("retry")}
          </Button>
        </div>
      ) : (
        data && (
          <>
            <p className="text-xs text-muted-foreground">
              {data.container ? t("container", { name: data.container.name, mode: data.container.networkMode, port: data.container.listenPort }) : t("noContainer")}
            </p>
            <p className="text-xs" role="status">
              {origin?.kind === "manual" ? t("originManual") : picked ? t("originDiscovered", { address: picked.address, source: t(`kinds.${picked.kind}`, { via: picked.via ?? "" }) }) : t("originNone")}
            </p>
            {candidates.length === 0 ? (
              <p className="text-sm text-muted-foreground">{t("none")}</p>
            ) : (
              <ul className="flex flex-col" aria-label={t("candidates")}>
                {candidates.map((c) => {
                  const current = c.address === address.trim();
                  return (
                    <li key={c.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-border/50 py-2 first:border-0">
                      <div className="min-w-0 flex-1">
                        <div className="flex flex-wrap items-center gap-2">
                          <span className="font-mono text-[13px]">{c.address}</span>
                          <ToneBadge tone={REACH_TONE[c.reach]}>{t(`reach.${c.reach}`)}</ToneBadge>
                          {c.recommended && <ToneBadge tone="accent">{t("recommended")}</ToneBadge>}
                        </div>
                        <p className="text-[11.5px] text-muted-foreground">
                          {t(`kinds.${c.kind}`, { via: c.via ?? "" })} · {t(`explain.${c.kind}`)}
                        </p>
                      </div>
                      <Button size="sm" variant={current ? "ghost" : "outline"} disabled={current} onClick={() => onPick(c)} aria-label={t("useAria", { address: c.address })}>
                        {current && <Check data-icon="inline-start" />}
                        {current ? t("inUse") : t("use")}
                      </Button>
                    </li>
                  );
                })}
              </ul>
            )}
          </>
        )
      )}
    </section>
  );
}
