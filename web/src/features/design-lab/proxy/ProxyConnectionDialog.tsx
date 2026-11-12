// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useState } from "react";
import { Check, Radar, Server } from "lucide-react";
import { toast } from "sonner";
import { useHosts } from "@/data/queries";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { FieldGroup } from "@/shared/ui/field";
import { FormSection, SelectField, TextField } from "./controls";
import { DISCOVERED_PROXIES, proxyStore, useProxy, validUrl, type ProxyConnection } from "./model";
import { useProxyText } from "./use-proxy-text";

export function ProxyConnectionDialog({ instance, onClose, onConfigured }: { instance?: string; onClose: () => void; onConfigured: (id: string) => void }) {
  const t = useProxyText();
  const { connections, instances } = useProxy();
  const hosts = useHosts();
  const current = instance ? connections[instance] : undefined;
  const [mode, setMode] = useState(current?.mode ?? "discovered");
  const [name, setName] = useState(current ? instances.find((p) => p.id === instance)?.name ?? "" : "");
  const [draft, setDraft] = useState<ProxyConnection>(current ?? { mode: "manual", container: "", host: instances.find((p) => p.id === instance)?.hostId ?? "atlas", image: "", configPath: "/etc/traefik/dynamic", apiUrl: "" });
  const [candidateId, setCandidateId] = useState(DISCOVERED_PROXIES.find((p) => p.host === draft.host && !Object.entries(connections).some(([id, c]) => id !== instance && c.host === p.host && c.container === p.container))?.id ?? "");
  const [error, setError] = useState("");
  const candidate = DISCOVERED_PROXIES.find((p) => p.id === candidateId);
  const patch = (value: Partial<ProxyConnection>) => setDraft((d) => ({ ...d, ...value }));
  function save() {
    const value = mode === "discovered" ? candidate : { ...draft, container: draft.container.trim(), configPath: draft.configPath.trim(), mode: "manual" as const };
    if (!value || !value.container || !value.configPath || (value.apiUrl && !validUrl(value.apiUrl))) { setError(t("setupValidation")); return; }
    if (Object.entries(connections).some(([id, c]) => id !== instance && c.host === value.host && c.container === value.container)) { setError(t("alreadyConfigured")); return; }
    const id = current && instance ? instance : instances.find((p) => p.hostId === value.host && !connections[p.id])?.id ?? crypto.randomUUID();
    const hostName = hosts.data?.find((h) => h.id === value.host)?.name ?? value.host;
    const label = name.trim() || "Traefik · " + hostName;
    proxyStore.set((s) => ({ ...s, instance: id, instances: s.instances.some((p) => p.id === id) ? s.instances.map((p) => p.id === id ? { id, name: label, hostId: value.host } : p) : [...s.instances, { id, name: label, hostId: value.host }], connections: { ...s.connections, [id]: value } }));
    onConfigured(id); onClose(); toast.success(t("setupSaved"));
  }
  return <Dialog open onOpenChange={(open) => !open && onClose()}><DialogContent className="flex max-h-[90dvh] flex-col gap-0 overflow-hidden p-0 sm:max-w-2xl">
    <DialogHeader className="border-b p-5 pr-12 text-left"><DialogTitle>{t(current ? "editInstance" : "addInstance")}</DialogTitle><DialogDescription>{t("connectionIntro")}</DialogDescription></DialogHeader>
    <form id="proxy-setup-form" className="min-h-0 flex-1 overflow-auto p-5" onSubmit={(e) => { e.preventDefault(); save(); }}><FieldGroup>
      <SegmentedToggle value={mode} onChange={(value) => { setMode(value as "discovered" | "manual"); setError(""); }} label={t("connectionMethod")} fit="content" wrap options={[{ value: "discovered", label: t("discovered"), icon: <Radar /> }, { value: "manual", label: t("manual"), icon: <Server /> }]} />
      {mode === "discovered" ? <div className="flex flex-col gap-3">{DISCOVERED_PROXIES.map((p) => {
        const used = Object.entries(connections).some(([id, c]) => id !== instance && c.host === p.host && c.container === p.container);
        return <button type="button" key={p.id} disabled={used} aria-pressed={candidateId === p.id} onClick={() => { setCandidateId(p.id); setError(""); }} className="flex items-center gap-3 rounded-xl border p-4 text-left transition-colors hover:bg-muted/40 aria-pressed:border-primary aria-pressed:bg-primary/10 disabled:cursor-not-allowed disabled:opacity-50"><IconTile tone="cyan"><Server /></IconTile><span className="min-w-0 flex-1"><strong className="block break-all text-sm">{p.container}</strong><span className="mt-1 block text-xs text-muted-foreground">{hosts.data?.find((h) => h.id === p.host)?.name ?? p.host} · {p.image}</span><span className="mt-2 block text-xs text-muted-foreground">{used ? t("alreadyConfigured") : p.configPath}</span></span>{candidateId === p.id && !used && <Check className="size-5 shrink-0 text-primary-text" />}</button>;
      })}<p className="text-xs text-muted-foreground">{t("discoveryReason")}</p></div> : <FormSection title={t("connectionSettings")} tone="target"><FieldGroup className="grid gap-4 sm:grid-cols-2"><TextField label={t("containerName")} value={draft.container} onChange={(container) => patch({ container })} required /><SelectField label={t("proxyHost")} value={draft.host} onChange={(host) => patch({ host })} options={(hosts.data ?? []).map((h) => ({ value: h.id, label: h.name }))} /></FieldGroup><TextField label={t("configDirectory")} value={draft.configPath} onChange={(configPath) => patch({ configPath })} required /><TextField label={t("apiUrl")} value={draft.apiUrl} onChange={(apiUrl) => patch({ apiUrl })} hint={t("apiHint")} /></FormSection>}
      <TextField label={t("instanceName")} value={name} onChange={setName} placeholder={t("instanceNameHint")} />
      {mode === "discovered" && candidate && <div className="flex flex-wrap gap-2"><ToneBadge tone="accent">{t("fileProvider")}</ToneBadge><ToneBadge tone="cyan">{t("dockerProvider")}</ToneBadge></div>}
      {error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    </FieldGroup></form>
    <DialogFooter className="border-t p-4"><Button variant="outline" onClick={onClose}>{t("cancel")}</Button><Button type="submit" form="proxy-setup-form" disabled={mode === "discovered" && !candidate}>{t(current ? "save" : "configureInstance")}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
