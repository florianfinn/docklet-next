// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation. M34 #549: hidden select labels, wide dialog and eligibility badge.

import { ToneBadge } from "@/shared/components/tone";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";
import { Checkbox } from "@/shared/ui/checkbox";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { PROXY_REASON_KEYS } from "@/data/wire-catalogue-proxy";
import { ProxyError } from "@/data/http/proxy-api";
import type { Acknowledgements, Eligibility, Inventory, Operation } from "./types";

export function Choice({
  id,
  label,
  value,
  onChange,
  options,
  placeholder,
  disabled,
  hideLabel,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: { value: string; label: string; disabled?: boolean }[];
  placeholder?: string;
  disabled?: boolean;
  hideLabel?: boolean;
}) {
  return (
    <div className="grid min-w-0 gap-2">
      <Label htmlFor={id} className={hideLabel ? "sr-only" : undefined}>{label}</Label>
      <Select value={value} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger id={id} className="w-full min-w-0 [&>span]:truncate">
          <SelectValue placeholder={placeholder} />
        </SelectTrigger>
        <SelectContent className="max-w-[calc(100vw-2rem)]">
          {options.map(o => (
            <SelectItem
              className="break-all [&>span]:whitespace-normal"
              key={o.value}
              value={o.value}
              disabled={o.disabled}
            >
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
export function ConfirmCheck({
  checked,
  onChange,
  children,
  disabled,
}: {
  checked: boolean;
  disabled?: boolean;
  onChange: (value: boolean) => void;
  children: ReactNode;
}) {
  return (
    <label className="flex items-start gap-3 text-sm leading-relaxed">
      <Checkbox disabled={disabled} checked={checked} onCheckedChange={v => onChange(v === true)} />
      <span>{children}</span>
    </label>
  );
}
export function ProxyDialog({
  title,
  description,
  onClose,
  children,
  busy = false,
  wide = false,
}: {
  title: string;
  description: string;
  onClose: () => void;
  children: ReactNode;
  busy?: boolean;
  wide?: boolean;
}) {
  return (
    <Dialog
      open
      onOpenChange={open => {
        if (!open && !busy) onClose();
      }}
    >
      <DialogContent
        className={`[&_button]:max-w-full [&_button]:whitespace-normal max-h-[calc(100dvh-2rem)] min-w-0 grid-cols-[minmax(0,1fr)] overflow-y-auto ${wide ? "sm:max-w-4xl" : "sm:max-w-2xl"}`}
        onEscapeKeyDown={e => {
          if (busy) e.preventDefault();
        }}
        onPointerDownOutside={e => {
          if (busy) e.preventDefault();
        }}
        showCloseButton={!busy}
      >
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}
export function Diagnostic({ value }: { value?: { path?: string; line?: number; detail?: string } }) {
  const t = useTranslations("proxyContract.ui");
  return value ? (
    // overflow-wrap:anywhere (not break-word) lowers min-content, so long Caddy JSON log
    // tokens cannot widen the dialog column on narrow screens.
    <div role="group" aria-label={t("diagnostic")} className="min-w-0 text-sm [overflow-wrap:anywhere]">
      <p>
        {value.path}
        {value.line ? `:${value.line}` : ""}
      </p>
      <pre className="whitespace-pre-wrap font-mono text-xs [overflow-wrap:anywhere]">{value.detail}</pre>
    </div>
  ) : null;
}
export function Reason({ code }: { code?: string }) {
  const t = useTranslations();
  if (!code) return null;
  const key = PROXY_REASON_KEYS[code as keyof typeof PROXY_REASON_KEYS];
  return <span>{key ? t(key) : t("proxyContract.ui.unknownReason", { code })}</span>;
}
export function EligibilityView({ value }: { value: Eligibility }) {
  const t = useTranslations("proxyContract.ui");
  return (
    <div className="space-y-1 text-sm">
      <p className="flex flex-wrap items-center gap-2">
        <ToneBadge tone={value.writable ? "ok" : "warn"}>{t(value.writable ? "writable" : "readOnly")}</ToneBadge> <Reason code={value.reason} />
      </p>
      <Diagnostic value={value} />
    </div>
  );
}
export function ErrorView({ error }: { error: unknown }) {
  const t = useTranslations("proxyContract.ui");
  return (
    <div role="alert" className="space-y-2 rounded-lg border border-destructive/40 p-3 text-sm">
      <p>{t(error instanceof Error && error.message === "proxy.invalid-response" ? "invalidResponse" : "error")}</p>
      {error instanceof ProxyError && (
        <>
          <Reason code={error.code} />
          <Diagnostic value={error.diagnostic} />
        </>
      )}
    </div>
  );
}
/** A definite refusal shown inside the open dialog so the draft can be corrected. */
export function RefusalView({ refusal }: { refusal?: unknown }) {
  const t = useTranslations("proxyContract.ui");
  if (refusal === undefined) return null;
  if (refusal instanceof Error) return <ErrorView error={refusal} />;
  const op = refusal as Operation;
  return (
    <div role="alert" className="space-y-2 rounded-lg border border-destructive/40 p-3 text-sm">
      <p>{t("refused")}</p>
      <Reason code={op.reason} />
      <Diagnostic value={op.diagnostic} />
    </div>
  );
}
export function AcknowledgementFields({
  inventory,
  unresolved = [],
  value,
  onChange,
}: {
  inventory: Inventory;
  unresolved?: string[];
  value: Acknowledgements;
  onChange: (v: Acknowledgements) => void;
}) {
  const t = useTranslations("proxyContract.ui");
  return (
    <div className="grid gap-3">
      {inventory.runningDiverged && (
        <>
          <p className="text-sm text-amber-700 dark:text-amber-300">{t("diverged")}</p>
          <ConfirmCheck
            checked={value.replaceRunningConfig}
            onChange={v => onChange({ ...value, replaceRunningConfig: v })}
          >
            {t("replaceAck")}
          </ConfirmCheck>
        </>
      )}
      {inventory.adminOff && (
        <ConfirmCheck checked={value.restart} onChange={v => onChange({ ...value, restart: v })}>
          {t("restartAck")}
        </ConfirmCheck>
      )}
      {unresolved.length > 0 && (
        <ConfirmCheck
          checked={unresolved.every(n => value.unresolvedReferences?.includes(n))}
          onChange={v => onChange({ ...value, unresolvedReferences: v ? unresolved : [] })}
        >
          {t("unresolvedAck", { names: unresolved.join(", ") })}
        </ConfirmCheck>
      )}
    </div>
  );
}
