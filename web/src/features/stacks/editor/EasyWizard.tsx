import { AlertCircle, Lock } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import type { Issue } from "@/domain/compose/validate";
import type { Mutator } from "@/domain/compose/ops";
import type { Host } from "@/domain/model";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription, AlertTitle } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";

import { useEditorText } from "./editor-text";
import { ReviewStep } from "./ReviewStep";
import { sectionFor, type SectionKey } from "./sections";
import { ServicesStep } from "./ServicesStep";
import { StackStep } from "./StackStep";
import type { ComposeDraft } from "./use-compose-draft";

export interface WizardState {
  step: 1 | 2 | 3;
  service: string | null;
  section: SectionKey;
}

interface EasyWizardProps {
  draft: ComposeDraft;
  wizard: WizardState;
  onWizard: (w: WizardState) => void;
  hosts: Host[];
  isNew: boolean;
  labelSuggestions: string[];
  edit: (fn: Mutator) => void;
  onEditError: (message: string) => void;
  onOpenRaw: () => void;
  deploy: ReactNode;
}

/** Nested Easy wizard: stack basics → services (with sections) → review. */
export function EasyWizard({ draft, wizard, onWizard, hosts, isNew, labelSuggestions, edit, onEditError, onOpenRaw, deploy }: EasyWizardProps) {
  const t = useTranslations("editor");
  const text = useEditorText();

  if (!draft.parsed.ok) {
    return (
      <Alert variant="destructive" className="border-destructive/30 bg-destructive/5">
        <Lock />
        <AlertTitle>{t("locked.title")}</AlertTitle>
        <AlertDescription className="flex flex-col items-start gap-2">
          <span>{t("locked.body")}</span>
          {draft.parsed.errors.slice(0, 3).map((e, i) => (
            <span key={i} className="font-mono text-xs">
              {text.parseError(e)}
            </span>
          ))}
          <Button size="sm" variant="outline" onClick={onOpenRaw}>
            {t("locked.fix")}
          </Button>
        </AlertDescription>
      </Alert>
    );
  }
  const model = draft.parsed.model;
  const errors = draft.issues.filter((i) => i.level === "error");
  const stepErr = (n: 1 | 2 | 3) => (n === 1 ? errors.some((i) => i.field === "name") : n === 2 ? errors.some((i) => !!i.service || i.field === "services") : false);
  const go = (step: WizardState["step"]) => onWizard({ ...wizard, step });
  const fix = (i: Issue) => {
    if (i.field === "name") onWizard({ ...wizard, step: 1 });
    else onWizard({ step: 2, service: i.service ?? null, section: sectionFor(i) });
  };
  const steps = [
    { n: 1 as const, label: t("steps.stack"), sub: t("steps.stackSub") },
    { n: 2 as const, label: t("steps.services"), sub: t("steps.servicesSub") },
    { n: 3 as const, label: t("steps.review"), sub: t("steps.reviewSub") },
  ];

  return (
    <div className="flex flex-col gap-4">
      <ol className="grid grid-cols-3 gap-2" aria-label={t("steps.label")}>
        {steps.map((s) => {
          const active = wizard.step === s.n;
          const err = stepErr(s.n);
          return (
            <li key={s.n}>
              <button
                type="button"
                aria-current={active ? "step" : undefined}
                onClick={() => go(s.n)}
                data-glow="row"
                className={cn(
                  "lift flex w-full items-center gap-2.5 rounded-xl border bg-card px-3 py-2.5 text-left text-muted-foreground outline-none hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50",
                  active && "border-control/55 bg-control/15 text-foreground",
                )}
              >
                <span
                  className={cn(
                    "grid size-7 shrink-0 place-items-center rounded-full bg-muted text-xs font-semibold transition-colors duration-(--dur)",
                    active && "bg-control text-control-foreground",
                    err && "bg-destructive/20 text-destructive",
                  )}
                >
                  {err ? <AlertCircle className="size-3.5" aria-label="!" /> : s.n}
                </span>
                <span className="flex min-w-0 flex-col leading-tight">
                  <strong className="text-sm font-medium">{s.label}</strong>
                  <small className="hidden truncate text-xs text-muted-foreground sm:block">{s.sub}</small>
                </span>
              </button>
            </li>
          );
        })}
      </ol>
      {wizard.step === 1 && <StackStep draft={draft} model={model} hosts={hosts} isNew={isNew} labelSuggestions={labelSuggestions} onEditError={onEditError} onNext={() => go(2)} />}
      {wizard.step === 2 && (
        <ServicesStep
          model={model}
          issues={draft.issues}
          selected={wizard.service}
          section={wizard.section}
          onSelect={(service) => onWizard({ ...wizard, service })}
          onSection={(section) => onWizard({ ...wizard, section })}
          edit={edit}
          onOpenRaw={onOpenRaw}
          onBack={() => go(1)}
          onNext={() => go(3)}
        />
      )}
      {wizard.step === 3 && (
        <ReviewStep
          model={model}
          issues={draft.issues}
          compose={draft.compose}
          savedCompose={draft.savedCompose}
          hostName={hosts.find((h) => h.id === draft.hostId)?.name ?? draft.hostId}
          onFix={fix}
          onBack={() => go(2)}
          deploy={deploy}
        />
      )}
    </div>
  );
}
