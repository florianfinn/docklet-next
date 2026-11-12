import { useState } from "react";
import { Check, ChevronsUpDown, Plus, Server, Settings2 } from "lucide-react";
import { useHosts } from "@/data/queries";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";
import { ProxyConnectionDialog } from "./ProxyConnectionDialog";
import { useProxy } from "./model";
import { useProxyText } from "./use-proxy-text";
export function ProxyInstances({ instance, onSelect, onSettings }: { instance: string; onSelect: (id: string) => void; onSettings: () => void }) {
  const t = useProxyText();
  const { instances, connections } = useProxy();
  const hosts = useHosts();
  const [open, setOpen] = useState(false);
  const [adding, setAdding] = useState(false);
  const active = instances.find((p) => p.id === instance);
  const connection = connections[instance];
  return <>
    <div className="mb-5 flex flex-wrap items-center gap-3 rounded-xl border bg-card p-3">
      <Popover open={open} onOpenChange={setOpen}><PopoverTrigger asChild><button aria-label={t("switchInstance")} className="flex w-full min-w-0 items-center gap-3 rounded-lg p-1 text-left outline-none transition-colors hover:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring sm:w-auto sm:flex-none sm:pr-4"><IconTile tone="accent"><Server /></IconTile><span className="min-w-0"><span className="block text-[11px] text-muted-foreground">{t("activeInstance")}</span><strong className="block truncate text-sm">{active?.name}</strong></span><ChevronsUpDown className="ml-3 size-4 shrink-0 text-muted-foreground" /></button></PopoverTrigger>
      <PopoverContent align="start" className="w-[min(28rem,calc(100vw-2rem))] p-2"><p className="px-3 py-2 text-xs font-medium text-muted-foreground">{t("instances")}</p><div className="max-h-80 overflow-y-auto">{[true, false].filter((configured) => instances.some((p) => !!connections[p.id] === configured)).map((configured) => <section key={String(configured)}><p className="px-3 pb-1 pt-3 text-[11px] text-muted-foreground">{t(configured ? "configuredInstances" : "availableInstances")}</p>{instances.filter((p) => !!connections[p.id] === configured).map((p) => <button key={p.id} onClick={() => { onSelect(p.id); setOpen(false); }} aria-current={p.id === instance ? "true" : undefined} className="flex w-full items-center gap-3 rounded-lg px-3 py-3 text-left hover:bg-muted/40 aria-[current=true]:bg-primary/10"><Server className="size-4 shrink-0 text-primary-text" /><span className="min-w-0 flex-1"><strong className="block truncate text-sm">{p.name}</strong><span className="mt-1 block truncate text-xs text-muted-foreground">{connections[p.id]?.container ?? t("setupNeeded")} · {hosts.data?.find((h) => h.id === p.hostId)?.name ?? p.hostId}</span></span>{p.id === instance && <Check className="size-4 shrink-0 text-primary-text" />}</button>)}</section>)}</div><div className="mt-2 border-t pt-2"><Button variant="ghost" className="w-full justify-start" onClick={() => { setOpen(false); setAdding(true); }}><Plus data-icon="inline-start" />{t("addInstance")}</Button></div></PopoverContent></Popover>
      <div className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1 sm:border-l sm:pl-4"><span className="break-all font-mono text-xs">{connection?.container ?? t("setupNeeded")}</span><ToneBadge tone={connection ? "cyan" : "gray"}>{hosts.data?.find((h) => h.id === (connection?.host ?? active?.hostId))?.name ?? active?.hostId}</ToneBadge></div>
      {connection && <Button variant="ghost" size="icon-sm" aria-label={t("connectionSettings")} onClick={onSettings}><Settings2 /></Button>}
    </div>
    {adding && <ProxyConnectionDialog onClose={() => setAdding(false)} onConfigured={onSelect} />}
  </>;
}
