import { useCapability } from "@/data/use-capability";
import { ArrowLeft, ArrowRight, Cable, Network, Plus, Router } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import type { Host, TopologyNetwork } from "@/domain/model";
import { canonicalIp6, formatCidr6, parseCidr6 } from "@/domain/ipv6";
import { CREATABLE_DRIVERS, hasErrors, validateNetworkDraft, validateNetworkEntries, type CreatableDriver, type NetworkDraft, type NetworkDraftErrors } from "@/domain/networks";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { cn } from "@/shared/lib/utils";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { HelpLabel, HelpTarget, HelpTooltip } from "@/shared/help/help-tooltip";
import { Field, FieldContent, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Spinner } from "@/shared/ui/spinner";
import { Switch } from "@/shared/ui/switch";

import { AdvancedNetworkFields, type NetworkEntry } from "./AdvancedNetworkFields";
import { HostScope } from "./HostScope";
import { useNetworkCommands } from "./use-network-commands";

const STEPS = ["kind", "settings", "review"] as const;
const canonicalCidr6 = (text: string) => {
  const c = parseCidr6(text);
  return c ? formatCidr6(c) : undefined;
};
type Step = (typeof STEPS)[number];
const DRIVER_ICON: Record<CreatableDriver, React.ReactNode> = { bridge: <Network />, macvlan: <Router />, ipvlan: <Cable /> };

const blank = (hostId: string): NetworkDraft => ({ hostId, name: "", driver: "bridge", ipv4: true, ipv6: false, subnet: "", gateway: "", ipRange: "", subnet6: "", gateway6: "", ipRange6: "", parent: "", internal: false, labels: {}, options: {}, auxAddresses: [], auxAddresses6: [] });

/** Switch of one address family; both sit in one row above every expandable field, so toggling never moves them. */
function FamilySwitch({ family, label, on, onToggle }: { family: "ipv4" | "ipv6"; label: string; on: boolean; onToggle: (on: boolean) => void }) {
  const id = `net-${family}`;
  return (
    <Field orientation="horizontal" className="w-auto" data-slot="family-anchor">
      <Switch id={id} checked={on} onCheckedChange={onToggle} aria-controls={`${id}-fields`} />
      <FieldLabel htmlFor={id} id={`${id}-label`} className="text-base font-semibold">
        {label}
      </FieldLabel>
    </Field>
  );
}

/** Fields of one address family below the switch row; an off family keeps a one-line note. */
function FamilySection({ family, on, offText, children }: { family: "ipv4" | "ipv6"; on: boolean; offText: string; children: React.ReactNode }) {
  const id = `net-${family}`;
  return (
    <section id={`${id}-fields`} aria-labelledby={`${id}-label`} data-family={family} data-on={on} className={cn("flex min-w-0 flex-col gap-4 rounded-xl border p-3.5", on ? "border-control/35 bg-control/5" : "bg-muted/10")}>
      {on ? children : <p className="text-xs text-muted-foreground">{offText}</p>}
    </section>
  );
}

interface Props {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  hosts: Host[];
  networks: TopologyNetwork[];
  defaultHostId?: string;
  onCreated: (network: TopologyNetwork) => void;
}

/** Network wizard: kind, settings, review. Validation is the shared domain rule; the adapter repeats it. */
export function CreateNetworkDialog({ open, onOpenChange, hosts, networks, defaultHostId, onCreated }: Props) {
  const t = useTranslations("networks.createDialog");
  const ts = useTranslations("networks.scope");
  const online = hosts.filter((h) => h.status === "online");
  const initialHost = online.find((h) => h.id === defaultHostId)?.id ?? online[0]?.id ?? "";
  const [draft, setDraft] = useState<NetworkDraft>(() => blank(initialHost));
  const cmd = useNetworkCommands(draft.hostId, open);
  const capability = useCapability("createNetwork", { kind: "host", id: draft.hostId });
  const [labels, setLabels] = useState<NetworkEntry[]>([]);
  const [options, setOptions] = useState<NetworkEntry[]>([]);
  const [advancedOpen, setAdvancedOpen] = useState(false);
  const [step, setStep] = useState<Step>("kind");
  const [tried, setTried] = useState(false);
  const [busy, setBusy] = useState(false);
  const patch = (p: Partial<NetworkDraft>) => setDraft((d) => ({ ...d, ...p }));
  const errors: NetworkDraftErrors = { ...validateNetworkDraft(draft, networks.filter((n) => n.hostId === draft.hostId)), labels: validateNetworkEntries(labels), options: validateNetworkEntries(options) };
  const lan = draft.driver !== "bridge";
  const { internal: bridgeInternal } = draft;
  const err = (k: keyof NetworkDraftErrors) => (tried && errors[k] ? t(`errors.${errors[k]}`) : undefined);
  const index = STEPS.indexOf(step);

  const close = (o: boolean) => {
    onOpenChange(o);
    if (!o) {
      setDraft(blank(initialHost));
      setLabels([]);
      setOptions([]);
      setAdvancedOpen(false);
      setStep("kind");
      setTried(false);
    }
  };

  const next = () => {
    if (step === "settings" && hasErrors(errors)) {
      setTried(true);
      if (errors.labels || errors.options || errors.auxAddresses || errors.auxAddresses6) setAdvancedOpen(true);
      return;
    }
    setStep(STEPS[index + 1] ?? "review");
  };

  const submit = async () => {
    setBusy(true);
    const created = await cmd.create({ ...draft, name: draft.name.trim() });
    setBusy(false);
    if (!created) return;
    toast.success(t("created", { name: created.name }));
    onCreated(created);
    close(false);
  };

  const v4 = draft.ipv4 !== false;
  const v6 = draft.ipv6 === true;
  const subnet6 = draft.subnet6?.trim() ?? "";
  const rows: [string, string][] = [
    [t("host"), hosts.find((h) => h.id === draft.hostId)?.name ?? draft.hostId],
    [t("driver"), draft.driver],
    [t("name"), draft.name.trim()],
    ...(lan ? ([[t("parent"), draft.parent.trim()]] as [string, string][]) : []),
    [t("families"), [v4 && "IPv4", v6 && "IPv6"].filter(Boolean).join(" + ")],
    ...(v4
      ? ([
          [t("subnet"), draft.subnet.trim() || t("autoSubnet")],
          [t("gateway"), draft.gateway.trim() || (draft.subnet.trim() ? t("none") : t("autoSubnet"))],
          [t("ipRange"), draft.ipRange.trim() || t("none")],
        ] as [string, string][])
      : []),
    ...(v6
      ? ([
          [t("subnet6"), (subnet6 && (canonicalCidr6(subnet6) ?? subnet6)) || t("autoSubnet")],
          [t("gateway6"), (draft.gateway6?.trim() && (canonicalIp6(draft.gateway6) ?? draft.gateway6.trim())) || (subnet6 ? t("none") : t("autoSubnet"))],
          [t("ipRange6"), draft.ipRange6?.trim() || t("none")],
        ] as [string, string][])
      : []),
    ...([["labels", draft.labels], ["options", draft.options]] as const).filter(([, value]) => Object.keys(value ?? {}).length).map(([key, value]): [string, string] => [t(`advanced.${key}`), Object.entries(value ?? {}).map(([k, v]) => `${k}=${v}`).join(", ")]),
    ...([["auxAddresses", v4 ? draft.auxAddresses : []], ["auxAddresses6", v6 ? draft.auxAddresses6 : []]] as const).filter(([, value]) => value?.length).map(([key, value]): [string, string] => [t(`advanced.${key}`), (value ?? []).map((a) => `${a.name.trim()}=${a.ip.trim()}`).join(", ")]),
    [t("internal"), draft.internal ? "✓" : "–"],
  ];

  const internalField = (
    <Field orientation="horizontal" className="self-start sm:pt-7">
      <Switch id="net-internal" checked={draft.internal} onCheckedChange={(internal) => patch({ internal })} />
      <FieldContent>
        <HelpLabel htmlFor="net-internal" topic="network.internal">
          {t("internal")}
        </HelpLabel>
        <FieldDescription>{t("internalHint")}</FieldDescription>
      </FieldContent>
    </Field>
  );

  return (
    <Dialog open={open} onOpenChange={close}>
      {/* Width and height are fixed from the start, so switching IPv4/IPv6 or steps never re-centres the dialog; only the body scrolls when the screen is short. */}
      <DialogContent className="grid h-[min(56rem,calc(100dvh-2rem))] grid-rows-[auto_auto_minmax(0,1fr)_auto] sm:max-w-[60rem]" data-surface="create-network-dialog">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
          <HostScope host={hosts.find((h) => h.id === draft.hostId)} hint={ts("create")} />
        </DialogHeader>
        <ol aria-label={t("stepsLabel")} className="flex gap-2 text-xs">
          {STEPS.map((s, i) => (
            <li key={s} aria-current={s === step ? "step" : undefined} className={cn("flex items-center gap-1.5 rounded-full border px-2.5 py-1", s === step ? "border-control/60 bg-control/15 font-medium text-control-text" : i < index ? "border-control/30 text-foreground" : "text-muted-foreground")}>
              <span className="font-mono tabular-nums">{i + 1}</span> {t(`steps.${s}`)}
            </li>
          ))}
        </ol>

        <div className="-mx-1 min-h-0 overflow-y-auto px-1 pb-1" data-slot="wizard-body">
        {step === "kind" && (
          <div className="flex flex-col gap-4">
            <Field>
              <FieldLabel htmlFor="net-host">{t("host")}</FieldLabel>
              <Select value={draft.hostId} onValueChange={(hostId) => patch({ hostId })}>
                <SelectTrigger id="net-host" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {hosts.map((h) => (
                      <SelectItem key={h.id} value={h.id} disabled={h.status !== "online"}>
                        <i className="size-2.5 rounded-[3px]" style={{ background: h.color }} aria-hidden />
                        {h.status === "online" ? h.name : t("hostOffline", { name: h.name })}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <div className="flex items-center gap-1 text-sm font-medium">
              {t("driver")}
              <HelpTooltip topic="network.driver" />
            </div>
            <ChoiceCards<CreatableDriver>
              label={t("driver")}
              value={draft.driver}
              onChange={(driver) => patch({ driver, internal: driver === "bridge" && bridgeInternal })}
              columns={3}
              options={CREATABLE_DRIVERS.map((d) => ({ value: d, title: t(`drivers.${d}.title`), description: t(`drivers.${d}.body`), icon: DRIVER_ICON[d] }))}
            />
            <p className="text-xs text-muted-foreground">{t("notCreatable")}</p>
          </div>
        )}

        {step === "settings" && (
          <div className="flex flex-col gap-5" data-slot="network-settings">
            {/* Fixed upper part: name, parent or internal, then both family switches; enabling a family only adds fields below the switch row. */}
            <div className="grid gap-4 sm:grid-cols-2">
              <TextField id="net-name" label={t("name")} help="network.name" value={draft.name} onChange={(name) => patch({ name })} description={t("nameHint")} error={err("name")} mono placeholder={lan ? "lan_iot" : "proxy_net"} />
              {lan ? <TextField id="net-parent" label={t("parent")} help="network.parent" value={draft.parent} onChange={(parent) => patch({ parent })} description={t("parentHint")} error={err("parent")} mono placeholder="br0" /> : internalField}
            </div>
            <fieldset className="flex flex-col gap-3" aria-describedby="net-families-hint">
              <legend className="mb-1 flex items-center gap-1 text-sm font-medium">
                {t("families")}
                <HelpTooltip topic="network.ipv6" />
              </legend>
              <p id="net-families-hint" className="text-xs text-muted-foreground">
                {t("familiesHint")} {t("ipv6Hint")}
              </p>
              {/* Reserved line: the error never pushes the family sections down. */}
              <p role="alert" className="min-h-5 text-sm text-destructive" data-slot="family-error">
                {err("family") ?? ""}
              </p>
              {/* Same two columns as the sections below, also on phones, so each switch keeps its place whatever is expanded. */}
              <div className="grid grid-cols-2 gap-4" data-slot="family-switches">
                <FamilySwitch family="ipv4" label={t("ipv4")} on={v4} onToggle={(ipv4) => patch({ ipv4 })} />
                <FamilySwitch family="ipv6" label={t("ipv6")} on={v6} onToggle={(ipv6) => patch({ ipv6 })} />
              </div>
              <div className="grid items-start gap-4 md:grid-cols-2">
                <FamilySection family="ipv4" on={v4} offText={t("familyOff", { family: "IPv4" })}>
                  <TextField id="net-subnet" label={t("subnet")} help="network.subnet" value={draft.subnet} onChange={(subnet) => patch({ subnet })} description={lan ? t("subnetHintLan") : t("subnetHintBridge")} error={err("subnet")} mono placeholder={lan ? "192.0.2.0/24" : "198.51.100.208/28"} />
                  <TextField id="net-gateway" label={t("gateway")} help="network.gateway" value={draft.gateway} onChange={(gateway) => patch({ gateway })} description={lan ? t("gatewayHintLan") : t("gatewayHintBridge")} error={err("gateway")} mono placeholder={lan ? "192.0.2.1" : "198.51.100.209"} />
                  <TextField id="net-range" label={t("ipRange")} help="network.ipRange" value={draft.ipRange} onChange={(ipRange) => patch({ ipRange })} description={t("ipRangeHint")} error={err("ipRange")} mono placeholder={lan ? "192.0.2.224/28" : ""} />
                </FamilySection>
                <FamilySection family="ipv6" on={v6} offText={t("familyOff", { family: "IPv6" })}>
                  <TextField id="net-subnet6" label={t("subnet6")} help="network.subnet6" value={draft.subnet6 ?? ""} onChange={(subnet6) => patch({ subnet6 })} description={lan ? t("subnet6HintLan") : t("subnet6HintBridge")} error={err("subnet6")} mono placeholder={lan ? "2001:db8:a::/64" : "2001:db8:d::/64"} />
                  <TextField id="net-gateway6" label={t("gateway6")} help="network.gateway6" value={draft.gateway6 ?? ""} onChange={(gateway6) => patch({ gateway6 })} description={lan ? t("gateway6HintLan") : t("gateway6HintBridge")} error={err("gateway6")} mono placeholder={lan ? "2001:db8:a::1" : "2001:db8:d::1"} />
                  <TextField id="net-range6" label={t("ipRange6")} help="network.ipRange6" value={draft.ipRange6 ?? ""} onChange={(ipRange6) => patch({ ipRange6 })} description={t("ipRange6Hint")} error={err("ipRange6")} mono placeholder={lan ? "2001:db8:a::100/120" : ""} />
                </FamilySection>
              </div>
            </fieldset>
            <AdvancedNetworkFields open={advancedOpen} onOpenChange={setAdvancedOpen} draft={draft} patch={patch} labels={labels} options={options} onLabelsChange={(entries) => { setLabels(entries); patch({ labels: Object.fromEntries(entries.map((e) => [e.key.trim(), e.value])) }); }} onOptionsChange={(entries) => { setOptions(entries); patch({ options: Object.fromEntries(entries.map((e) => [e.key.trim(), e.value])) }); }} error={err} />
          </div>
        )}

        {step === "review" && (
          <div className="flex flex-col gap-3">
            <section className="rounded-xl border bg-muted/15 p-3" aria-label={t("review")}>
              <dl className="grid grid-cols-1 gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_minmax(0,1fr)]">
                {rows.map(([k, v]) => (
                  <div key={k} className="contents">
                    <dt className="text-muted-foreground">{k}</dt>
                    <dd className="min-w-0 font-mono text-[13px] break-all">{v}</dd>
                  </div>
                ))}
              </dl>
            </section>
            {lan && (
              <Alert>
                <Router />
                <AlertDescription>{t("lanWarning")}</AlertDescription>
              </Alert>
            )}
          </div>
        )}

        </div>

        <DialogFooter className="sm:justify-between">
          <Button type="button" variant="outline" disabled={index === 0 || busy} onClick={() => setStep(STEPS[index - 1] ?? "kind")}>
            <ArrowLeft data-icon="inline-start" />
            {t("back")}
          </Button>
          {step === "review" ? (
            <HelpTarget topic="network.review">
              <Button type="button" disabled={busy || !capability.available || hasErrors(errors)} title={capability.reason} onClick={() => void submit()}>
                {busy ? <Spinner data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
                {t("submit")}
              </Button>
            </HelpTarget>
          ) : (
            <Button type="button" disabled={!draft.hostId || !capability.available} title={capability.reason} onClick={next}>
              {t("next")}
              <ArrowRight data-icon="inline-end" />
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
