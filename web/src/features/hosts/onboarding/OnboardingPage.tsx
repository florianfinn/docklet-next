import { ArrowLeft, ArrowRight, Lock, Server, Trash2 } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { useTranslations } from "use-intl";

import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { useConnectedSettings } from "@/data/queries";
import { agentHubUrl } from "@/domain/settings";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { PageHeader, QueryError } from "@/shared/components/page";
import { StepRail } from "@/shared/components/step-rail";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";
import { Spinner } from "@/shared/ui/spinner";

import { useHostCommands } from "../use-host-commands";
import { ConnectionStep } from "./ConnectionStep";
import { ConnectStep, InstallStep } from "./ConnectSteps";
import { DirectoriesStep } from "./DirectoriesStep";
import { draftFileOwner, freshDraft, ONBOARDING_STEPS, onboardingStore, patchDraft, planFromDraft, useOnboardingDraft, validateStep, type OnboardingStep } from "./onboarding-draft";

/** Four-step host onboarding; credentials are discarded when the page closes. */
export default function OnboardingPage() {
  return <CapabilityGate operation="registerHost"><OnboardingFlow /></CapabilityGate>;
}

function OnboardingFlow() {
  const t = useTranslations("hosts.onboarding");
  const settings = useConnectedSettings();
  const adapter = useAdapter();
  const discovery = useQuery({ queryKey: ["hub", "discovery"], queryFn: () => adapter.discoverHub(), enabled: adapter.kind === "http" });
  useEffect(() => () => onboardingStore.set(null), []);
  const hosts = useHostScope();
  const cmd = useHostCommands();
  const draft = useOnboardingDraft();
  const [params, setParams] = useSearchParams();
  const [attempted, setAttempted] = useState<OnboardingStep[]>([]);
  const [busy, setBusy] = useState(false);
  const [discard, setDiscard] = useState(false);

  const defaults = settings.data?.hostDefaults;
  useEffect(() => {
    if (!draft && defaults && !hosts.isPending) onboardingStore.set(freshDraft(defaults, hosts.all.map((h) => h.color)));
  }, [draft, defaults, hosts.isPending, hosts.all]);

  const requested = params.get("step") as OnboardingStep | null;
  const reached = draft?.reached ?? 0;
  const index = Math.min(Math.max(0, ONBOARDING_STEPS.indexOf(requested ?? "connection")), reached);
  const step = ONBOARDING_STEPS[index] ?? "connection";
  const locked = !!draft?.hostId;
  const registered = hosts.all.find((h) => h.id === draft?.hostId);
  const taken = useMemo(() => hosts.all.filter((h) => h.id !== draft?.hostId).map((h) => h.name), [hosts.all, draft?.hostId]);
  const errors = draft ? validateStep(step, draft, taken) : {};
  const general = settings.data?.general;
  const plan = draft && general ? planFromDraft(draft, draft.hubUrl ?? discovery.data?.candidates?.[0]?.url ?? agentHubUrl(general), general.timeZone) : undefined;

  if (plan && discovery.data?.agentImage) plan.image = discovery.data.agentImage;

  const go = (next: OnboardingStep) => setParams({ step: next });

  const onNext = async () => {
    if (!draft || !plan) return;
    if (!locked && Object.keys(errors).length) {
      setAttempted((a) => [...new Set([...a, step])]);
      return;
    }
    if (step === "directories" && !locked) {
      setBusy(true);
      const registration = await cmd.register({
        name: draft.name.trim(),
        color: draft.color,
        connection: draft.connection,
        address: `${draft.address.trim()}:${draft.port}`,
        projectDir: plan.projectDir,
        installDir: plan.installDir,
        fileOwner: draftFileOwner(draft),
        socketGid: plan.socketGid,
      });
      setBusy(false);
      if (!registration) return;
      patchDraft({ hostId: registration.host.id, token: registration.token, hubUrl: plan.hubUrl });
    }
    const nextIndex = index + 1;
    patchDraft({ reached: Math.max(reached, nextIndex) });
    const next = ONBOARDING_STEPS[nextIndex];
    if (next) go(next);
  };

  const steps = ONBOARDING_STEPS.map((id) => ({ id, title: t(`steps.${id}.title`), description: t(`steps.${id}.description`) }));

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="group mb-3 -ml-2 text-muted-foreground">
        <Link to="/hosts">
          <ArrowLeft data-icon="inline-start" className="transition-transform duration-(--dur) group-hover:-translate-x-0.5" />
          {t("toHosts")}
        </Link>
      </Button>
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          draft && (
            <Button variant="ghost" size="sm" className="text-muted-foreground" onClick={() => setDiscard(true)}>
              <Trash2 data-icon="inline-start" />
              {t("discard")}
            </Button>
          )
        }
      />
      {settings.isError || discovery.isError || hosts.isError ? (
        <QueryError error={settings.error ?? discovery.error ?? hosts.error} onRetry={() => void Promise.allSettled([settings.refetch(), discovery.refetch(), hosts.refetch()])} />
      ) : !draft || !plan || (adapter.kind === "http" && !discovery.data) ? (
        <div className="grid gap-5 lg:grid-cols-[15rem_1fr]">
          <Skeleton className="h-56 rounded-2xl" />
          <Skeleton className="h-[28rem] rounded-2xl" />
        </div>
      ) : (
        <div className="grid max-w-[82rem] grid-cols-[minmax(0,1fr)] items-start gap-5 lg:grid-cols-[15rem_minmax(0,1fr)]">
          <div className="lg:sticky lg:top-20">
            <StepRail steps={steps} current={step} reachable={reached} onSelect={(id) => go(id as OnboardingStep)} label={t("railLabel")} />
          </div>
          <Card className="gap-0 overflow-hidden p-0">
            <div className="flex items-center gap-3 border-b px-5 py-4">
              <span className="grid size-9 place-items-center rounded-xl text-white" style={{ background: draft.color }} aria-hidden>
                <Server className="size-4" />
              </span>
              <div className="min-w-0">
                <h2 className="text-lg font-semibold tracking-tight">{t(`steps.${step}.title`)}</h2>
                <p className="truncate text-xs text-muted-foreground">{draft.name.trim() || t("namePlaceholder")}</p>
              </div>
            </div>
            <div key={step} className="flex flex-col gap-4 p-5">
              {locked && (step === "connection" || step === "directories") && (
                <Alert>
                  <Lock />
                  <AlertDescription className="text-xs">{t("locked")}</AlertDescription>
                </Alert>
              )}
              {step === "connection" && <ConnectionStep draft={draft} errors={errors} showErrors={attempted.includes(step)} hubUrl={plan.hubUrl} candidates={discovery.data?.candidates} locked={locked} />}
              {step === "directories" && <DirectoriesStep draft={draft} errors={errors} showErrors={attempted.includes(step)} locked={locked} />}
              {step === "install" && <InstallStep draft={draft} plan={plan} />}
              {step === "connect" && <ConnectStep draft={draft} host={registered} hubUrl={plan.hubUrl} />}
            </div>
            <footer className="flex flex-wrap items-center gap-2 border-t bg-muted/20 px-5 py-3">
              <Button variant="outline" disabled={index === 0} onClick={() => go(ONBOARDING_STEPS[index - 1] ?? "connection")}>
                <ArrowLeft data-icon="inline-start" />
                {t("back")}
              </Button>
              <span className="flex-1" />
              {attempted.includes(step) && Object.keys(errors).length > 0 && !locked && (
                <span role="alert" className="text-xs text-destructive">
                  {t("fixErrors", { count: Object.keys(errors).length })}
                </span>
              )}
              {step !== "connect" && (
                <Button onClick={() => void onNext()} disabled={busy || (step === "install" && !draft.token)}>
                  {busy ? <Spinner data-icon="inline-start" /> : null}
                  {step === "directories" && !locked ? (busy ? t("registering") : t("register")) : t("next")}
                  {!busy && <ArrowRight data-icon="inline-end" />}
                </Button>
              )}
            </footer>
          </Card>
        </div>
      )}
      <ConfirmDialog
        open={discard}
        onOpenChange={setDiscard}
        title={t("discardTitle")}
        description={t("discardBody")}
        confirmLabel={t("discardConfirm")}
        destructive
        onConfirm={() => {
          onboardingStore.set(null);
          setAttempted([]);
          go("connection");
        }}
      />
    </>
  );
}
