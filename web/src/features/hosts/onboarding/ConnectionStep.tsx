import { ArrowDownLeft, ArrowUpRight, Check, ShieldAlert } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { FieldGroup, FieldLegend, FieldSet } from "@/shared/ui/field";

import type { HubDiscovery } from "@/domain/hub-discovery";

import { HOST_COLORS, patchDraft, type DraftErrors, type HostDraft } from "./onboarding-draft";
import { useFieldError } from "./use-field-error";

export function ConnectionStep({ draft, errors, showErrors, hubUrl, candidates, locked }: { draft: HostDraft; errors: DraftErrors; showErrors: boolean; hubUrl: string; candidates?: HubDiscovery["candidates"]; locked: boolean }) {
  const t = useTranslations("hosts.onboarding");
  const err = useFieldError(errors, showErrors);
  return (
    <FieldGroup>
      <div className="grid gap-4 sm:grid-cols-[1fr_auto]">
        <TextField id="host-name" label={t("name")} value={draft.name} onChange={(name) => patchDraft({ name })} placeholder={t("namePlaceholder")} description={t("nameHint")} error={err("name")} disabled={locked} />
        <FieldSet className="gap-2">
          <FieldLegend variant="label" className="mb-1.5">
            {t("color")}
          </FieldLegend>
          <div role="radiogroup" aria-label={t("color")} className="flex flex-wrap gap-1.5">
            {HOST_COLORS.map((c) => (
              <button
                key={c}
                type="button"
                role="radio"
                aria-checked={draft.color === c}
                aria-label={t("colorOption", { color: c })}
                disabled={locked}
                onClick={() => patchDraft({ color: c })}
                className={cn("grid size-7 place-items-center rounded-lg ring-offset-2 ring-offset-background outline-none transition-transform focus-visible:ring-2 focus-visible:ring-ring active:scale-90", draft.color === c && "ring-2 ring-foreground/60")}
                style={{ background: c }}
              >
                {draft.color === c && <Check className="size-3.5 text-white" strokeWidth={3} />}
              </button>
            ))}
          </div>
        </FieldSet>
      </div>
      <FieldSet>
        <FieldLegend variant="label">{t("connectionLabel")}</FieldLegend>
        <fieldset disabled={locked} className="contents">
          <ChoiceCards
            label={t("connectionLabel")}
            value={draft.connection}
            onChange={(connection) => patchDraft({ connection })}
            options={[
              {
                value: "direct",
                title: t("directTitle"),
                description: t("directDesc"),
                icon: <ArrowUpRight />,
                badge: <ToneBadge tone="accent" className="h-5 px-1.5 text-[10.5px]">{t("directBadge")}</ToneBadge>,
              },
              { value: "edge", title: t("edgeTitle"), description: t("edgeDesc"), icon: <ArrowDownLeft /> },
            ]}
          />
        </fieldset>
      </FieldSet>
      {draft.connection === "direct" ? (
        <div className="grid gap-4 sm:grid-cols-[1fr_8rem]">
          <TextField id="host-address" label={t("address")} value={draft.address} onChange={(address) => patchDraft({ address })} placeholder={t("addressPlaceholder")} description={t("addressHint")} error={err("address")} mono disabled={locked} />
          <TextField id="host-port" label={t("port")} value={draft.port} onChange={(port) => patchDraft({ port })} error={err("port")} mono disabled={locked} />
        </div>
      ) : (
        <div className="rounded-xl border bg-card/40 p-3.5">
          <TextField id="host-hub-url" label={t("hubUrl")} value={hubUrl} onChange={(hubUrl) => patchDraft({ hubUrl })} error={err("hubUrl")} mono disabled={locked} />
          {candidates && <div className="mt-2 flex flex-col items-start gap-1" aria-label={t("hubCandidates")}>
            {candidates.map((candidate) => <button type="button" key={candidate.id} disabled={locked} className="max-w-full text-left text-xs break-words text-primary-text hover:underline" onClick={() => patchDraft({ hubUrl: candidate.url })}>
              {candidate.url} · {t(candidate.reach === "same-host" ? "reachSameHost" : "reachUnknown")}
            </button>)}
          </div>}
          <p className="mt-1 text-xs text-muted-foreground">
            {t("hubUrlHint")} {t("proxyBypass")}{" "}
            <Link to="/settings/general" className="text-primary-text hover:underline">
              {t("hubUrlEdit")}
            </Link>
          </p>
        </div>
      )}
      {(draft.connection === "direct" || hubUrl.startsWith("http://")) && <Alert className="border-warning/30 bg-warning/8"><ShieldAlert /><AlertDescription>{t("httpWarning")}</AlertDescription></Alert>}
      <p className="flex items-start gap-2 text-xs text-muted-foreground">
        <ShieldAlert className="mt-0.5 size-3.5 shrink-0" aria-hidden />
        {t("noWireguard")}
      </p>
    </FieldGroup>
  );
}
