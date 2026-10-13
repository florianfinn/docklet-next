import { ArrowLeft, ArrowRight, LogIn, TriangleAlert, Waypoints } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { Link, Navigate, useNavigate, useSearchParams } from "react-router";
import { useTranslations } from "use-intl";

import { LanguageSwitch } from "@/app/i18n/LanguageSwitch";
import { useSettings } from "@/data/queries";
import { DesignLabPanel } from "@/features/design-lab/DesignLabPanel";
import { useMotionEnabled } from "@/shared/theme/use-design-tokens";
import { READABLE_PLATE } from "@/features/setup/readable-plate";
import { BrandLogo } from "@/shared/brand/BrandLogo";
import { BusyButton } from "@/shared/components/busy-button";
import { useBusyAction } from "@/shared/components/use-busy-action";
import { TextField } from "@/shared/components/form-field";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";
import { DotField } from "@/shared/visual/DotField";

import { parseVariantChoice, resolveVariant, simulateLogin, validateLogin, type ForcedOutcome, type LocalLogin, type LoginFailure, type VariantChoice } from "@/features/auth/login/login-model";

/** "none" is no page to show: it redirects, so the demo offers the variants that have a form. */
const VARIANT_CHOICES: VariantChoice[] = ["settings", "local", "proxy"];
const OUTCOMES: ForcedOutcome[] = ["none", "invalid", "unavailable", "locked"];

/**
 * Full-screen login demo for the login modes of the hub settings. The form
 * validates and simulates the check; it never authenticates, stores or sends
 * the password, and success only navigates to the dashboard. No route is
 * guarded by it. With the effective mode "none" there is nothing to log in
 * to: /login goes straight to the dashboard. The demo variant comes from
 * ?variant= (design lab › Abläufe), so local and proxy stay reachable when
 * the hub runs without login.
 */
export default function LoginPage() {
  const t = useTranslations("login");
  const navigate = useNavigate();
  const motion = useMotionEnabled();
  const settings = useSettings();
  const [params, setParams] = useSearchParams();
  const choice = parseVariantChoice(params.get("variant"));
  const setChoice = (v: VariantChoice) => setParams(v === "settings" ? {} : { variant: v }, { replace: true });
  const [forced, setForced] = useState<ForcedOutcome>("none");
  const [form, setForm] = useState<LocalLogin>({ username: "", password: "" });
  const [attempted, setAttempted] = useState(false);
  const [failure, setFailure] = useState<LoginFailure | null>(null);
  const abort = useRef<AbortController | null>(null);
  useEffect(() => () => abort.current?.abort(), []);

  const auth = settings.data?.auth;
  const waiting = choice === "settings" && settings.isPending;
  const variant = resolveVariant(choice, auth);
  const errors = validateLogin(form);
  const shown = attempted ? errors : {};

  const login = useBusyAction(async () => {
    setFailure(null);
    abort.current?.abort();
    abort.current = new AbortController();
    try {
      const result = await simulateLogin(variant, form, { auth, forced, signal: abort.current.signal });
      if (result.ok) {
        setForm({ username: form.username, password: "" });
        void navigate("/dashboard");
      } else setFailure(result.reason);
    } catch (e) {
      if (!(e instanceof DOMException && e.name === "AbortError")) throw e;
    }
  });

  const edit = (patch: Partial<LocalLogin>) => {
    setForm((f) => ({ ...f, ...patch }));
    setFailure(null);
  };
  const submitLocal = () => {
    setAttempted(true);
    if (Object.keys(errors).length) return;
    void login.run();
  };
  const failureText = failure && (variant === "proxy" && failure === "unavailable" ? t("errors.proxyUnavailable") : t(`errors.${failure}`));
  if (!waiting && variant === "none") return <Navigate to="/dashboard" replace />;

  return (
    <div className="dot-grid flex min-h-svh flex-col" data-slot="login-page">
      <DotField fixed />
      <div className="mx-auto flex w-full max-w-md flex-1 flex-col justify-center gap-5 px-4 py-8">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <ToneBadge tone="violet" className={cn(READABLE_PLATE, "rounded-full")}>
            {t("badge")}
          </ToneBadge>
          <div className="flex flex-wrap items-center gap-2">
            <LanguageSwitch size="xs" className={cn(READABLE_PLATE, "py-1 pr-1 pl-2")} />
            <Button asChild variant="outline" size="sm">
              <Link to="/dashboard">
                <ArrowLeft data-icon="inline-start" />
                {t("exit")}
              </Link>
            </Button>
          </div>
        </div>
        <Card className="gap-5 p-6" aria-labelledby="login-title">
          <div className="flex flex-col items-center gap-3 text-center">
            <BrandLogo size={56} entranceSlot="login" motion={motion} />
            <h1 id="login-title" className="text-xl font-semibold tracking-tight">
              {t("title", { hub: settings.data?.general.hubName ?? "Docklet" })}
            </h1>
          </div>
          {waiting ? (
            <div className="flex flex-col gap-3" aria-busy="true" aria-label={t("loading")}>
              <Skeleton className="h-9 rounded-md" />
              <Skeleton className="h-9 rounded-md" />
              <Skeleton className="h-9 rounded-md" />
            </div>
          ) : (
            <>
              {choice === "settings" && settings.isError && (
                <Alert className="border-warning/40 bg-warning/8">
                  <TriangleAlert className="text-warning" />
                  <AlertDescription className="flex flex-col items-start gap-2">
                    {t("settingsError")}
                    <Button size="sm" variant="outline" onClick={() => void settings.refetch()}>
                      {t("retry")}
                    </Button>
                  </AlertDescription>
                </Alert>
              )}
              {failureText && (
                <Alert variant="destructive" className="border-destructive/30 bg-destructive/5" data-slot="login-error">
                  <TriangleAlert />
                  <AlertDescription>{failureText}</AlertDescription>
                </Alert>
              )}
              {variant === "local" && (
                <form
                  className="flex flex-col gap-4"
                  noValidate
                  onSubmit={(e) => {
                    e.preventDefault();
                    submitLocal();
                  }}
                >
                  <TextField id="login-user" label={t("username")} value={form.username} onChange={(username) => edit({ username })} autoComplete="username" error={shown.username && t("required")} />
                  <TextField id="login-password" type="password" label={t("password")} value={form.password} onChange={(password) => edit({ password })} autoComplete="current-password" error={shown.password && t("required")} />
                  <BusyButton type="submit" busy={login.busy} busyLabel={t("submitting")} icon={<LogIn data-icon="inline-start" />} className="w-full">
                    {t("submit")}
                  </BusyButton>
                  <p className="text-xs text-muted-foreground">{t("localHint", { user: auth?.local.username ?? "admin" })}</p>
                </form>
              )}
              {variant === "proxy" && (
                <div className="flex flex-col gap-4">
                  <p className="flex gap-2.5 text-sm text-muted-foreground">
                    <Waypoints className="mt-0.5 size-4 shrink-0 text-info" aria-hidden />
                    <span>{t("proxyLead", { header: auth?.proxy.userHeader ?? "Remote-User", proxies: auth?.proxy.trustedProxies || "–" })}</span>
                  </p>
                  <BusyButton busy={login.busy} busyLabel={t("proxySubmitting")} icon={<ArrowRight data-icon="inline-start" />} className="w-full" onClick={() => void login.run()}>
                    {t("proxySubmit")}
                  </BusyButton>
                </div>
              )}
            </>
          )}
        </Card>
        <section aria-labelledby="login-demo" className={cn(READABLE_PLATE, "flex flex-col gap-3 p-4")} data-slot="login-demo-controls">
          <div>
            <h2 id="login-demo" className="text-sm font-semibold">
              {t("demo.title")}
            </h2>
            <p className="text-xs text-muted-foreground">{t("demo.hint")}</p>
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">{t("demo.variant")}</span>
            <SegmentedToggle<VariantChoice>
              value={choice}
              onChange={(v) => {
                setChoice(v);
                setFailure(null);
              }}
              label={t("demo.variant")}
              size="sm"
              fit="content"
              wrap
              options={VARIANT_CHOICES.map((v) => ({ value: v, label: t(`demo.variants.${v}`) }))}
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted-foreground">{t("demo.outcome")}</span>
            <SegmentedToggle<ForcedOutcome> value={forced} onChange={setForced} label={t("demo.outcome")} size="sm" fit="content" wrap options={OUTCOMES.map((o) => ({ value: o, label: t(`demo.outcomes.${o}`) }))} />
          </div>
          <p className="text-xs text-muted-foreground">{t("demo.noneHint")}</p>
        </section>
      </div>
      <DesignLabPanel />
    </div>
  );
}
