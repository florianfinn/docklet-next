// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original runtime edit form using fresh configuration and per-intent retry keys.

import { useQuery } from "@tanstack/react-query";
import { useState, useSyncExternalStore } from "react";
import { useTranslations } from "use-intl";
import { identityLifetime } from "@/data/identity-lifetime";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { ContainerInfo } from "@/domain/model";
import { imageRef } from "@/domain/updates";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Textarea } from "@/shared/ui/textarea";
import { QueryError } from "@/shared/components/page";
import { recreateDraft, recreateEdit, recreateIntent, type EditConfig } from "./recreate-edit";
import { RecreateRun } from "./RecreateRun";

export function RecreateDialog({ container, open, onOpenChange }: { container: ContainerInfo | undefined; open: boolean; onOpenChange: (o: boolean) => void }) {
  const t = useTranslations("containers.recreate");
  return <Dialog open={open} onOpenChange={onOpenChange}>
    {container && open && <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
      <DialogHeader><DialogTitle>{t("title", { name: container.name })}</DialogTitle><DialogDescription>{t("description")}</DialogDescription></DialogHeader>
      <LoadForm key={container.id} c={container} close={() => onOpenChange(false)} />
    </DialogContent>}
  </Dialog>;
}

function LoadForm({ c, close }: { c: ContainerInfo; close: () => void }) {
  const adapter = useAdapter();
  const lifetime = identityLifetime(adapter);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const t = useTranslations("containers.recreate");
  const capability = useCapability("recreateContainer", { kind: "container", id: c.id });
  const config = useQuery({ queryKey: ["container-edit", c.id, generation], staleTime: 0, retry: false,
    queryFn: () => adapter.getContainerEditConfig ? adapter.getContainerEditConfig(c.id) : Promise.resolve<EditConfig>({ id: c.id, name: c.name, image: imageRef(c.image), running: c.state === "running", environment: Object.entries(c.env).map(([key, value]) => `${key}=${value}`), hostConfig: { restartPolicy: { name: c.restartPolicy }, portBindings: Object.fromEntries(c.ports.map((port) => [`${port.container}/${port.protocol ?? "tcp"}`, port.host ? [{ hostPort: port.host }] : []])) } }),
    enabled: capability.available,
  });
  if (!capability.available) return <p role="status">{capability.reason}</p>;
  if (config.isError) return <QueryError error={config.error} onRetry={() => void config.refetch()} />;
  if (!config.data) return <p role="status">{t("loading")}</p>;
  return <EditForm key={generation} c={c} config={config.data} close={close} />;
}

function EditForm({ c, config, close }: { c: ContainerInfo; config: EditConfig; close: () => void }) {
  const t = useTranslations("containers.recreate");
  const tc = useTranslations("common.actions");
  const cmd = useContainerCommands();
  const capability = useCapability("recreateContainer", { kind: "container", id: c.id });
  const [initial] = useState(() => recreateDraft(config));
  const [draft, setDraft] = useState(initial);
  const [intent] = useState(() => recreateIntent());
  const [pending, setPending] = useState(false);
  const [invalid, setInvalid] = useState(false);
  const [runId, setRunId] = useState<string>();
  const patch = (value: Partial<typeof draft>) => { setDraft((old) => ({ ...old, ...value })); setInvalid(false); };
  const submit = async () => {
    let edit;
    try { edit = recreateEdit(initial, draft); } catch { setInvalid(true); return; }
    setPending(true);
    try { const id = await cmd.recreate(c.id, edit, undefined, intent(edit)); if (id) setRunId(id); }
    finally { setPending(false); }
  };
  if (runId) return <><RecreateRun hostId={c.hostId} runId={runId} onNavigated={close} /><DialogFooter><Button variant="outline" onClick={close}>{tc("close")}</Button></DialogFooter></>;
  return <>
    <Alert><AlertDescription>{t("warning")}</AlertDescription></Alert>
    <FieldGroup>
      <Field data-disabled={pending}><FieldLabel htmlFor="rc-name">{t("name")}</FieldLabel><Input id="rc-name" value={draft.name} disabled={pending} onChange={(event) => patch({ name: event.target.value })} /></Field>
      <Field data-disabled={pending}><FieldLabel htmlFor="rc-image">{t("image")}</FieldLabel><Input id="rc-image" value={draft.image} disabled={pending} onChange={(event) => patch({ image: event.target.value })} /></Field>
      <Field data-disabled={pending}><FieldLabel htmlFor="rc-restart">{t("restart")}</FieldLabel>
        <Select value={draft.restart} disabled={pending} onValueChange={(restart) => patch({ restart })}>
          <SelectTrigger id="rc-restart"><SelectValue /></SelectTrigger>
          <SelectContent><SelectGroup>{["no", "always", "on-failure", "unless-stopped"].map((value) => <SelectItem key={value} value={value}>{value}</SelectItem>)}</SelectGroup></SelectContent>
        </Select>
      </Field>
      <Field data-disabled={pending} data-invalid={invalid}><FieldLabel htmlFor="rc-ports">{t("ports")}</FieldLabel><Textarea id="rc-ports" aria-invalid={invalid} className="font-mono text-xs" rows={4} value={draft.ports} disabled={pending} onChange={(event) => patch({ ports: event.target.value })} /><FieldDescription>{t("portsHint")}</FieldDescription>{invalid && <FieldError>{t("invalidPorts")}</FieldError>}</Field>
      <Field data-disabled={pending}><FieldLabel htmlFor="rc-env">{t("env")}</FieldLabel><Textarea id="rc-env" className="font-mono text-xs" rows={4} value={draft.environment} disabled={pending} onChange={(event) => patch({ environment: event.target.value })} /><FieldDescription>{t("envHint")}</FieldDescription></Field>
    </FieldGroup>
    <DialogFooter><Button variant="ghost" disabled={pending} onClick={close}>{tc("cancel")}</Button><Button disabled={pending || !capability.available || !!c.inventory?.stale || !draft.name.trim() || !draft.image.trim()} title={capability.reason} onClick={() => void submit()}>{t("submit")}</Button></DialogFooter>
  </>;
}
