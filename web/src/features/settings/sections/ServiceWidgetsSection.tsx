// Source: docklet-next/docs/design/service-widgets.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original Settings › Service widgets section for issues #332 and #333.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { CircleCheck, ShieldCheck, TriangleAlert } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { useServiceWidgetConfig, useStacks } from "@/data/queries";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { useCommandError } from "@/data/use-command-error";
import { CREDENTIAL_KIND, SERVICE_KINDS, type ServiceKind, type WidgetConfig, type WidgetConfigInput, type WidgetResult } from "@/domain/service-widgets";
import { widgetErrorKey } from "@/features/dashboard/services/widget-meta";
import { TextField } from "@/shared/components/form-field";
import { QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { Field, FieldContent, FieldDescription, FieldGroup, FieldLabel } from "@/shared/ui/field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Skeleton } from "@/shared/ui/skeleton";
import { Switch } from "@/shared/ui/switch";

import { SettingsForm } from "../SettingsForm";
import { useSectionDraft } from "../use-section-draft";

const NO_STACK = "none";

/** Draft of one widget: the stored values plus a write-only credential field. */
type Draft = Omit<WidgetConfigInput, "credentialAction" | "credential"> & { credential: string; clearCredential: boolean };

function draftOf(config: WidgetConfig): Draft {
  return { enabled: config.enabled, address: config.address, openUrl: config.openUrl, stackId: config.stackId, tlsPin: config.tlsPin, credential: "", clearCredential: false };
}

function inputOf(draft: Draft): WidgetConfigInput {
  const { credential, clearCredential, ...rest } = draft;
  return { ...rest, ...(credential ? { credentialAction: "replace", credential } : { credentialAction: clearCredential ? "clear" : "keep" }) };
}

function TestResult({ result, onTrust }: { result: WidgetResult; onTrust: () => void }) {
  const t = useTranslations("settings.serviceWidgets");
  const td = useTranslations("dashboard.services");
  if (!result.error) {
    return (
      <p className="flex items-center gap-1.5 text-sm text-success" role="status">
        <CircleCheck className="size-4" aria-hidden />
        {t("testOk")}
      </p>
    );
  }
  const certificate = result.error.certificate;
  return (
    <div className="flex flex-col gap-2" role="status">
      <p className="flex items-center gap-1.5 text-sm text-destructive">
        <TriangleAlert className="size-4" aria-hidden />
        {td(widgetErrorKey(result.error))}
      </p>
      {certificate && (
        <div className="flex flex-col gap-2 rounded-lg border p-3 text-xs">
          <p>{t("certificate", { hostPort: certificate.hostPort })}</p>
          <code className="font-mono break-all" data-fingerprint>{certificate.sha256}</code>
          <Button type="button" size="sm" variant="outline" className="self-start" onClick={onTrust}>
            <ShieldCheck data-icon="inline-start" />
            {t("trust")}
          </Button>
        </div>
      )}
    </div>
  );
}

function WidgetForm({ kind, config }: { kind: ServiceKind; config: WidgetConfig }) {
  const t = useTranslations("settings.serviceWidgets");
  const td = useTranslations("dashboard.services");
  const adapter = useAdapter();
  const client = useQueryClient();
  const commandError = useCommandError();
  const stacks = useStacks();
  const { draft, patch, dirty, reset, commit } = useSectionDraft(draftOf(config));
  // Successful saves so far; a test started before a save never answers for the saved form.
  const [saves, setSaves] = useState(0);
  const [tested, setTested] = useState<{ input: string; saves: number; result: WidgetResult } | undefined>();
  const test = useMutation({
    mutationFn: (input: WidgetConfigInput) => adapter.testServiceWidget(kind, input),
    onMutate: () => { setTested(undefined); return saves; },
    onSuccess: (result, input, started) => setTested({ input: JSON.stringify(input), saves: started, result }),
  });
  if (!draft) return null;
  // A test answer belongs to the draft it tested: a late answer, an edit, discard or save hides it.
  const current = JSON.stringify(inputOf(draft));
  const result = tested?.input === current && tested.saves === saves ? tested.result : undefined;
  const testFailed = test.isError && test.context === saves && JSON.stringify(test.variables) === current;
  const credential = CREDENTIAL_KIND[kind];
  const id = (field: string) => `sw-${kind}-${field}`;
  return (
    <SettingsForm
      title={td(`${kind}.name`)}
      description={td(`${kind}.desc`)}
      dirty={dirty}
      onDiscard={reset}
      onSave={async () => {
        const submitted = draft;
        const saved = await adapter.saveServiceWidgetConfig(kind, inputOf(submitted));
        // The submitted credential leaves the form; later typing is kept.
        commit(draftOf(saved), submitted);
        setSaves((n) => n + 1);
        void client.invalidateQueries({ queryKey: queryKeys.serviceWidgets });
      }}
    >
      <FieldGroup data-widget-settings={kind}>
        <Field orientation="horizontal">
          <Switch id={id("enabled")} checked={draft.enabled} onCheckedChange={(enabled) => patch({ enabled })} />
          <FieldContent>
            <FieldLabel htmlFor={id("enabled")}>{t("enabled")}</FieldLabel>
            <FieldDescription>{t("enabledHint")}</FieldDescription>
          </FieldContent>
        </Field>
        <TextField id={id("address")} label={t("address")} value={draft.address} onChange={(address) => patch({ address })} description={t(kind === "minecraft" ? "addressHintMinecraft" : "addressHint")} placeholder={t(kind === "minecraft" ? "addressExampleMinecraft" : "addressExample")} mono />
        {credential && (
          <TextField
            id={id("credential")}
            type="password"
            autoComplete="off"
            label={t(credential)}
            value={draft.credential}
            onChange={(value) => patch({ credential: value, clearCredential: false })}
            description={config.hasCredential && !draft.clearCredential ? t("credentialStored") : t("credentialHint")}
            trailing={config.hasCredential && !draft.clearCredential && !draft.credential ? (
              <Button type="button" size="sm" variant="ghost" onClick={() => patch({ clearCredential: true })}>{t("clearCredential")}</Button>
            ) : undefined}
          />
        )}
        <TextField id={id("open")} label={t("openUrl")} value={draft.openUrl} onChange={(openUrl) => patch({ openUrl })} description={t("openUrlHint")} placeholder={t("openUrlExample")} mono />
        <Field>
          <FieldLabel htmlFor={id("stack")}>{t("stack")}</FieldLabel>
          <Select value={draft.stackId ?? NO_STACK} onValueChange={(value) => patch({ stackId: value === NO_STACK ? undefined : value })}>
            <SelectTrigger id={id("stack")} className="w-full sm:w-80">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={NO_STACK}>{t("noStack")}</SelectItem>
              {(stacks.data ?? []).map((stack) => (
                <SelectItem key={stack.id} value={stack.id}>{stack.name}</SelectItem>
              ))}
            </SelectContent>
          </Select>
          <FieldDescription>{t("stackHint")}</FieldDescription>
        </Field>
        {draft.tlsPin && (
          <div className="flex flex-wrap items-center gap-2 rounded-lg border p-3 text-xs" data-pin>
            <ShieldCheck className="size-4 text-success" aria-hidden />
            <span className="min-w-0 flex-1">{t("pinned", { hostPort: draft.tlsPin.hostPort })}</span>
            <Button type="button" size="sm" variant="ghost" onClick={() => patch({ tlsPin: undefined })}>{t("removePin")}</Button>
          </div>
        )}
        <div className="flex flex-col gap-2">
          <Button type="button" variant="outline" className="self-start" disabled={!draft.address.trim() || test.isPending} onClick={() => test.mutate(inputOf(draft))}>
            {test.isPending ? t("testing") : t("test")}
          </Button>
          {testFailed && <p className="text-sm text-destructive" role="status">{commandError(test.error)}</p>}
          {result && <TestResult result={result} onTrust={() => result.error?.certificate && patch({ tlsPin: result.error.certificate })} />}
        </div>
      </FieldGroup>
    </SettingsForm>
  );
}

function WidgetSettings({ kind }: { kind: ServiceKind }) {
  const config = useServiceWidgetConfig(kind);
  const retry = () => void config.refetch();
  if (!config.data) return config.isError ? <QueryError error={config.error} onRetry={retry} /> : <Skeleton className="h-96 rounded-2xl" />;
  // A failed refresh keeps the mounted form and its unsaved draft; the error shows above it.
  return (
    <div className="flex flex-col gap-2">
      {config.isError && <QueryError compact error={config.error} onRetry={retry} />}
      <WidgetForm kind={kind} config={config.data} />
    </div>
  );
}

/** Administrators configure each widget kind; the hub fetches and never returns a stored credential. */
export function ServiceWidgetsSection() {
  const t = useTranslations("settings.serviceWidgets");
  return (
    <div className="flex flex-col gap-5">
      <p className="max-w-5xl text-sm text-muted-foreground">{t("intro")}</p>
      {SERVICE_KINDS.map((kind) => <WidgetSettings key={kind} kind={kind} />)}
    </div>
  );
}
