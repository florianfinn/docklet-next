import { isHostAvailable } from "@/domain/hosts";
import { DialogCommandFooter } from "@/shared/components/dialog-command";
import { useDialogCommand } from "@/shared/components/use-dialog-command";
import { useNetworkCapabilities } from "./use-network-capabilities";
import { AlertTriangle, ArrowLeft, Ban, Link2, Unlink } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import type { ContainerInfo, Host, Topology } from "@/domain/model";
import { attachmentsOf, checkConnect, checkDisconnect, hasIpv4, sortNetworks, type ConnectCheck, type DisconnectCheck } from "@/domain/networks";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { TextField } from "@/shared/components/form-field";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { HelpLabel } from "@/shared/help/help-tooltip";
import { Field, FieldLabel } from "@/shared/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Spinner } from "@/shared/ui/spinner";

import { HostScope } from "./HostScope";
import { useNetworkCommands } from "./use-network-commands";

export type ConnectMode = "connect" | "disconnect";

export interface ConnectRequest {
  mode: ConnectMode;
  containerId?: string;
  networkId?: string;
}

interface Props {
  request: ConnectRequest;
  onClose: () => void;
  host: Host;
  topology: Topology;
  containers: ContainerInfo[];
}

const label = (c: Pick<ContainerInfo, "name" | "serviceName" | "stackId" | "stackName">) => (c.stackId && c.serviceName ? `${c.stackName ?? c.stackId} · ${c.serviceName}` : c.name);

/**
 * Connect or disconnect wizard for one host: pick container and network,
 * review the checked result, then confirm. Choices that Docker would refuse
 * stay visible but disabled; the adapter repeats every check.
 */
export function ConnectNetworkDialog({ request, onClose, host, topology, containers }: Props) {
  const t = useTranslations("networks.connectDialog");
  const ts = useTranslations("networks.scope");
  const feedback = useDialogCommand(true, host.id);
  const close = () => { feedback.dismiss(); onClose(); };
  const cmd = useNetworkCommands(host.id);
  const [mode, setMode] = useState<ConnectMode>(request.mode);
  const [containerId, setContainerId] = useState(request.containerId ?? "");
  const [networkId, setNetworkId] = useState(request.networkId ?? "");
  const [ip, setIp] = useState("");
  const [ip6, setIp6] = useState("");
  const [aliasesText, setAliasesText] = useState("");
  const aliases = aliasesText.trim() ? aliasesText.split(",").map((a) => a.trim()) : [];
  const [reviewing, setReviewing] = useState(false);
  const [busy, setBusy] = useState(false);

  const own = containers.filter((c) => c.hostId === host.id).toSorted((a, b) => label(a).localeCompare(label(b)));
  const nets = sortNetworks(topology.networks.filter((n) => n.hostId === host.id));
  const capabilities = useNetworkCapabilities(mode === "connect" ? "connectNetwork" : "disconnectNetwork", nets.map((n) => n.id));
  const container = own.find((c) => c.id === containerId);
  const network = nets.find((n) => n.id === networkId);
  const attached = new Set(container ? attachmentsOf(topology, container.id).map((a) => a.networkId) : []);
  const choices = mode === "connect" ? nets : nets.filter((n) => attached.has(n.id));
  const pickable = mode === "connect" ? own : own.filter((c) => attachmentsOf(topology, c.id).length > 0);

  const connect: ConnectCheck | undefined = mode === "connect" && container && network ? checkConnect(topology, container, network, ip, ip6, aliases) : undefined;
  const disconnect: DisconnectCheck | undefined = mode === "disconnect" && container && network ? checkDisconnect(topology, container, network) : undefined;
  const block = connect?.block ?? disconnect?.block;
  const ipError = connect?.ipError;
  const ip6Error = connect?.ip6Error;
  const pool = container && network && mode === "connect" ? checkConnect(topology, container, network) : undefined;
  const poolIp = pool?.ip;
  const staticPossible = !block && !!network?.subnetConfigured && !!network.subnet;
  const static6Possible = !block && !!network?.ipv6 && !!network.subnet6Configured && !!network.subnet6;
  // Both sides must belong to the shown host; the identity colour is no substitute for this id check.
  const ready = isHostAvailable(host) && !!capabilities.get(networkId)?.available && !!container && !!network && container.hostId === host.id && network.hostId === host.id && !block && !ipError && !ip6Error && !connect?.aliasError;
  const nameOf = (id: string) => {
    const c = containers.find((x) => x.id === id);
    return c ? label(c) : id;
  };

  const confirm = async () => {
    if (!container || !network || !ready) return;
    setBusy(true);
    const ok = await feedback.run(() => mode === "connect" ? cmd.connect({ networkId: network.id, containerId: container.id, ip: ip.trim() || undefined, ip6: ip6.trim() || undefined, aliases: connect?.aliases ?? [] }) : cmd.disconnect(network.id, container.id));
    setBusy(false);
    if (!ok) return;
    toast.success(t(mode === "connect" ? "connected" : "disconnected", { container: label(container), network: network.name }));
    close();
  };

  const warnings = connect?.warnings ?? disconnect?.warnings ?? [];

  return (
    <Dialog open onOpenChange={(o) => !o && close()}>
      <DialogContent {...feedback.bindings} className="max-h-[calc(100dvh-2rem)] grid-cols-[minmax(0,1fr)] overflow-y-auto sm:max-w-xl" data-surface="connect-network-dialog">
        <DialogHeader>
          <DialogTitle>{mode === "connect" ? t("titleConnect") : t("titleDisconnect")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
          <HostScope host={host} hint={mode === "connect" ? ts("connect") : ts("disconnect")} />
        </DialogHeader>
        {!reviewing ? (
          <div className="flex flex-col gap-4">
            <SegmentedToggle<ConnectMode>
              value={mode}
              onChange={(m) => {
                setMode(m);
                setNetworkId("");
                setIp("");
                setIp6("");
                setAliasesText("");
              }}
              label={t("mode")}
              options={(["connect", "disconnect"] as const).map((m) => ({ value: m, label: t(`modes.${m}`), icon: m === "connect" ? <Link2 /> : <Unlink /> }))}
              className="self-start"
            />
            <Field>
              <HelpLabel htmlFor="cn-container" topic={mode === "connect" ? "connect.container" : "connect.disconnect"}>
                {t("container")}
              </HelpLabel>
              <Select
                value={containerId}
                onValueChange={(id) => {
                  setContainerId(id);
                  if (mode === "disconnect") setNetworkId("");
                }}
              >
                <SelectTrigger id="cn-container" className="w-full min-w-0">
                  <SelectValue placeholder={t("containerPlaceholder")} />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {pickable.map((c) => (
                      <SelectItem key={c.id} value={c.id}>
                        {label(c)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Field>
              <FieldLabel htmlFor="cn-network">{t("network")}</FieldLabel>
              <Select value={networkId} onValueChange={setNetworkId} disabled={!container}>
                <SelectTrigger id="cn-network" className="w-full min-w-0">
                  <SelectValue placeholder={choices.length ? t("networkPlaceholder") : t("noOptions")}><span className="truncate">{network?.name}</span></SelectValue>
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {choices.map((n) => {
                      const blocked = container && mode === "connect" ? checkConnect(topology, container, n).block : undefined;
                      return (
                        <SelectItem key={n.id} value={n.id} disabled={!!n.detailUnavailable || !!n.inventory?.stale || !!blocked || !capabilities.get(n.id)?.available}>
                          <span className="font-medium">{n.name}</span>
                          <span className="font-mono text-xs text-muted-foreground">
                            {n.driver}
                            {n.subnet ? ` · ${n.subnet}` : ""}
                            {n.subnet6 ? ` · ${n.subnet6}` : ""}
                          </span>
                        </SelectItem>
                      );
                    })}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            {mode === "connect" && network && hasIpv4(network) && (
              <TextField
                id="cn-ip"
                label={t("ip")}
                help="connect.ip"
                value={ip}
                onChange={setIp}
                mono
                disabled={!staticPossible}
                placeholder={poolIp ?? ""}
                description={!staticPossible ? t("ipUnsupported") : poolIp ? t("ipHint", { ip: poolIp }) : t("ipHintNone")}
                error={ipError ? t(`ipErrors.${ipError}`, { subnet: network.subnet ?? "" }) : undefined}
              />
            )}
            {mode === "connect" && network?.ipv6 && (
              <TextField
                id="cn-ip6"
                label={t("ip6")}
                value={ip6}
                onChange={setIp6}
                mono
                disabled={!static6Possible}
                placeholder={pool?.ip6 ?? ""}
                description={!static6Possible ? t("ip6Unsupported") : pool?.ip6 ? t("ipHint", { ip: pool.ip6 }) : t("ipHintNone")}
                error={ip6Error ? t(`ip6Errors.${ip6Error}`, { subnet: network.subnet6 ?? "" }) : undefined}
              />
            )}
            {mode === "connect" && (
              <TextField id="cn-aliases" label={t("aliases")} description={t("aliasesHint")} value={aliasesText} onChange={setAliasesText} error={connect?.aliasError ? t("aliasesInvalid") : undefined} mono />
            )}
          </div>
        ) : (
          container &&
          network && (
            <div className="flex flex-col gap-3">
              <section className="rounded-xl border bg-muted/15 p-3" aria-label={t("summary")}>
                <dl className="grid grid-cols-1 gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_minmax(0,1fr)]">
                  <dt className="text-muted-foreground">{t("mode")}</dt>
                  <dd>{t(`modes.${mode}`)}</dd>
                  <dt className="text-muted-foreground">{t("container")}</dt>
                  <dd className="font-mono text-[13px] break-all">{label(container)}</dd>
                  <dt className="text-muted-foreground">{t("network")}</dt>
                  <dd className="font-mono text-[13px] break-all">
                    {network.name} · {network.driver}
                  </dd>
                  {mode === "connect" && (
                    <>
                      <dt className="text-muted-foreground">{t("address")}</dt>
                      <dd className="font-mono text-[13px]">{!hasIpv4(network) ? t("noIpv4") : ip.trim() || (connect?.ip ? `${connect.ip} (${t("addressPool")})` : t("addressPool"))}</dd>
                    </>
                  )}
                  {mode === "connect" && !!connect?.aliases?.length && (
                    <>
                      <dt className="text-muted-foreground">{t("aliases")}</dt>
                      <dd className="min-w-0 font-mono text-[13px] break-all">{connect.aliases.join(", ")}</dd>
                    </>
                  )}
                  {mode === "connect" && network.ipv6 && (
                    <>
                      <dt className="text-muted-foreground">{t("address6")}</dt>
                      <dd className="font-mono text-[13px] break-all">{connect?.ip6 ? (ip6.trim() ? connect.ip6 : `${connect.ip6} (${t("addressPool")})`) : t("addressPool")}</dd>
                    </>
                  )}
                </dl>
              </section>
              {warnings.length > 0 && (
                <ul className="flex flex-col gap-1.5" aria-label={t("summary")}>
                  {warnings.map((w) => (
                    <li key={w} className="flex gap-2 text-sm text-warning">
                      <AlertTriangle className="mt-0.5 size-4 shrink-0" aria-hidden />
                      <span>{w === "dependents" ? t("warnings.dependents", { names: (disconnect?.dependents ?? []).map(nameOf).join(", "), count: disconnect?.dependents.length ?? 0 }) : t(`warnings.${w}`)}</span>
                    </li>
                  ))}
                </ul>
              )}
            </div>
          )
        )}
        {block && <Alert variant="destructive"><Ban /><AlertDescription>{t(`blocks.${block}`)}</AlertDescription></Alert>}
        <DialogCommandFooter feedback={feedback} className="sm:justify-between">
          {reviewing ? (
            <Button type="button" variant="outline" disabled={busy} onClick={() => setReviewing(false)}>
              <ArrowLeft data-icon="inline-start" />
              {t("back")}
            </Button>
          ) : (
            <span />
          )}
          {reviewing ? (
            <Button type="button" variant={mode === "disconnect" ? "destructive" : "default"} disabled={!ready || busy} onClick={() => void confirm()}>
              {busy ? <Spinner data-icon="inline-start" /> : mode === "connect" ? <Link2 data-icon="inline-start" /> : <Unlink data-icon="inline-start" />}
              {mode === "connect" ? t("confirmConnect") : t("confirmDisconnect")}
            </Button>
          ) : (
            <Button type="button" disabled={!ready} onClick={() => setReviewing(true)}>
              {t("check")}
            </Button>
          )}
        </DialogCommandFooter>
      </DialogContent>
    </Dialog>
  );
}
