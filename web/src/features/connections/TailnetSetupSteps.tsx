// Source: docklet-next/issues/586
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original Tailscale setup steps (policy, credential, setup test) for M37 #586.

import { isCancelledError } from "@tanstack/react-query";
import { ChevronRight, Copy } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import { AdapterOutcomeError } from "@/data/capability";
import { useCommandError } from "@/data/use-command-error";
import { wireKey } from "@/data/wire-catalogue";
import { TAILNET_CODE_KEYS, TAILNET_TEST_CHECK_KEYS, TAILNET_TEST_STATE_KEYS } from "@/data/wire-catalogue-tailnet";
import { policyParts, type TailnetApi, type TailnetSettings, type TestResult } from "@/domain/tailnet";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";

/** One value to paste elsewhere: shown apart from the prose, with its own copy button. */
export function CopyValue({ label, value, onCopied }: { label: string; value: string; onCopied: (text: string) => void }) {
  const t = useTranslations("connections.steps");
  return <div className="flex min-w-0 flex-wrap items-center gap-2" data-slot="copy-value">
    <span className="w-full text-xs text-muted-foreground sm:w-36 sm:shrink-0">{label}</span>
    <code className="min-w-0 flex-1 rounded-md bg-muted/50 px-2 py-1 font-mono text-xs break-all">{value}</code>
    <Button type="button" variant="ghost" size="icon" className="size-10 shrink-0" aria-label={t("copyValue", { label, value })}
      onClick={() => void navigator.clipboard.writeText(value).then(() => onCopied(t("copiedValue", { label })), () => onCopied(t("copyFailed")))}>
      <Copy aria-hidden />
    </Button>
  </div>;
}

/** A numbered step: one sentence, the place to go, then the values; the badge comes from stored state or a passed test. */
export function Step({ title, where, hint, done, children }: { title: string; where?: string; hint?: string; done?: "setUp" | "verified"; children?: ReactNode }) {
  const t = useTranslations("connections.steps");
  return <section className="flex min-w-0 flex-col gap-2" data-slot="tailnet-step">
    <div className="flex flex-wrap items-center gap-2"><h4 className="text-sm font-semibold">{title}</h4>{done && <ToneBadge tone="ok">{t(done)}</ToneBadge>}</div>
    {where && <p className="flex min-w-0 flex-wrap items-center gap-1 text-xs font-medium" data-slot="step-where">{where.split(" → ").map((part, i) => <span key={i} className="inline-flex items-center gap-1">{i > 0 && <ChevronRight className="size-3 text-muted-foreground" aria-hidden />}<span className="rounded bg-muted/50 px-1.5 py-0.5">{part}</span></span>)}</p>}
    {hint && <p className="text-sm text-muted-foreground">{hint}</p>}
    {children}
  </section>;
}

export function PolicyStep({ settings, done, onCopied }: { settings: TailnetSettings; done?: "setUp"; onCopied: (text: string) => void }) {
  const t = useTranslations("connections");
  const s = useTranslations("connections.steps");
  const parts = policyParts(settings.policySnippet);
  return <Step title={t("policy")} where={settings.customControl ? s("policyWhereCustom") : s("policyWhere")} hint={t("policyHint")} done={done}>
    <div className="flex flex-wrap gap-2">
      <Button type="button" variant="outline" className="min-h-10" onClick={() => void navigator.clipboard.writeText(settings.policySnippet).then(() => onCopied(t("copied")), () => onCopied(t("copyFailed")))}>{t("copy")}</Button>
      {!settings.customControl && <Button asChild variant="outline" className="min-h-10"><a href="https://login.tailscale.com/admin/acls/file" target="_blank" rel="noreferrer">{s("openPolicy")}</a></Button>}
    </div>
    <pre className="max-w-full rounded-lg bg-muted/40 p-3 font-mono text-xs whitespace-pre-wrap break-all">{settings.policySnippet}</pre>
    {parts && !settings.customControl && <Collapsible>
      <CollapsibleTrigger asChild><Button type="button" variant="ghost" className="min-h-10 justify-start px-0">{s("formsToggle")}</Button></CollapsibleTrigger>
      <CollapsibleContent className="flex flex-col gap-2 pt-1">
        <p className="text-xs text-muted-foreground">{s("formsTags")}</p>
        {parts.tags.map((tag) => <CopyValue key={tag} label={s("tagName")} value={tag.replace(/^tag:/, "")} onCopied={onCopied} />)}
        <CopyValue label={s("tagOwner")} value={parts.owner} onCopied={onCopied} />
        <p className="pt-2 text-xs text-muted-foreground">{s("formsGrant")}</p>
        <CopyValue label={s("source")} value={parts.src} onCopied={onCopied} />
        <CopyValue label={s("destination")} value={parts.dst} onCopied={onCopied} />
        <CopyValue label={s("port")} value={parts.port} onCopied={onCopied} />
      </CollapsibleContent>
    </Collapsible>}
  </Step>;
}

export function CredentialStep({ settings, mode, done, onCopied }: { settings: TailnetSettings; mode: "oauth" | "manual"; done?: "setUp"; onCopied: (text: string) => void }) {
  const t = useTranslations("connections");
  const s = useTranslations("connections.steps");
  const tags = policyParts(settings.policySnippet)?.tags ?? [];
  if (mode === "manual") return <Step title={t("createManual")} where={settings.customControl ? undefined : s("manualWhere")} hint={t("createManualHint")} done={done}>
    {tags[0] && <CopyValue label={s("tags")} value={tags[0]} onCopied={onCopied} />}
    {!settings.customControl && <Button asChild variant="outline" className="min-h-10 self-start"><a href="https://login.tailscale.com/admin/settings/keys" target="_blank" rel="noreferrer">{t("console")}</a></Button>}
  </Step>;
  return <Step title={t("createOAuth")} where={s("oauthWhere")} hint={t("createOAuthHint")} done={done}>
    <CopyValue label={s("scope")} value={s("scopeValue")} onCopied={onCopied} />
    {tags.map((tag) => <CopyValue key={tag} label={s("tags")} value={tag} onCopied={onCopied} />)}
    <Button asChild variant="outline" className="min-h-10 self-start"><a href="https://login.tailscale.com/admin/settings/oauth" target="_blank" rel="noreferrer">{t("console")}</a></Button>
  </Step>;
}

type TestView = { epoch: number; result?: TestResult; error?: string; stale?: boolean };

/**
 * Runs the hub's setup test. A result shows only for the test epoch it started in (local save, reset, reconnect) and the
 * credential revision it proved; a revision observed later (for example a replacement from another tab) hides it.
 */
export function TestStep({ api, saved, revision, manual }: { api: TailnetApi; saved: boolean; revision: string | undefined; manual: boolean }) {
  const t = useTranslations("connections.steps");
  const all = useTranslations();
  const commandError = useCommandError();
  const [view, setView] = useState<TestView>();
  const [busy, setBusy] = useState(false);
  const current = view && view.epoch === api.testEpoch() && (!view.result || view.result.credentialRevision === revision) ? view : undefined;
  const checks = current?.result?.checks ?? [];
  const verified = checks.some((c) => c.state === "pass") && checks.every((c) => c.state === "pass" || c.state === "skipped");
  const passed = checks.filter((c) => c.state === "pass").length, failed = checks.filter((c) => c.state === "fail" || c.state === "unknown").length;
  async function run() {
    const epoch = api.testEpoch();
    setBusy(true); setView(undefined);
    try { setView({ epoch, result: await api.test() }); }
    catch (error) {
      if (isCancelledError(error)) setView({ epoch: api.testEpoch(), stale: true });
      else {
        const key = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? wireKey(TAILNET_CODE_KEYS, error.outcome.code) : undefined;
        setView({ epoch, error: key ? all(key) : commandError(error) });
      }
    } finally { setBusy(false); }
  }
  // The explicit test does not retry on its own; the generic provider text describes background provisioning.
  const codeText = (code: NonNullable<TestResult["checks"][number]["code"]>) => code === "tailnet.provider-unavailable" ? t("testProviderUnavailable") : all(TAILNET_CODE_KEYS[code]);
  return <Step title={t("testTitle")} hint={t(manual ? "testHintManual" : "testHint")} done={verified ? "verified" : undefined}>
    <Button type="button" className="min-h-10 self-start" disabled={!saved || busy} aria-busy={busy || undefined} onClick={() => void run()}>{t(busy ? "testRunning" : "testRun")}</Button>
    {!saved && <p className="text-xs text-muted-foreground">{t("testSaveFirst")}</p>}
    {current?.stale && <p role="status" className="text-sm">{t("testStale")}</p>}
    {current?.error && <p role="alert" className="text-sm text-destructive">{current.error}</p>}
    {checks.length > 0 && <p role="status" className="text-sm">{t("testDone", { passed, failed })}</p>}
    {checks.length > 0 && <ul className="grid gap-1.5 text-sm" aria-label={t("testTitle")} data-slot="tailnet-test-checks">
      {checks.map((check) => <li key={check.id} className="flex min-w-0 flex-wrap items-center gap-2" data-check={check.id} data-state={check.state}>
        <ToneBadge tone={check.state === "pass" ? "ok" : check.state === "fail" ? "err" : "gray"}>{all(TAILNET_TEST_STATE_KEYS[check.state])}</ToneBadge>
        <span>{all(TAILNET_TEST_CHECK_KEYS[check.id])}</span>
        {check.code && <span className="w-full text-xs text-muted-foreground">{codeText(check.code)}</span>}
        {check.providerStatus !== undefined && <span className="w-full text-xs text-muted-foreground">{all("connections.providerStatus", { status: check.providerStatus })}</span>}
        {check.id === "cleanup" && check.state === "unknown" && <span className="w-full text-xs text-muted-foreground">{t("cleanupUnknown")}</span>}
      </li>)}
    </ul>}
  </Step>;
}
