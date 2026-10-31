import { isHostAvailable } from "@/domain/hosts";
export { PruneDialog } from "./PruneDialog";
import { COMMAND_DIALOG_CONTENT } from "@/shared/components/dialog-layout";
import { sameImageReference } from "@/domain/docker-reference";
import { useCapability } from "@/data/use-capability";
import { useRegistries } from "@/features/registries/use-registries";
import { registryHost, registryReference } from "@/domain/registries";
import { useImageHostCapabilities } from "./use-image-capabilities";
import { DialogCommandFooter } from "@/shared/components/dialog-command";
import { useDialogCommand } from "@/shared/components/use-dialog-command";
import { Download } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";
import { formatBytes } from "@/domain/format";
import { useTranslations } from "use-intl";

import { parseImageReference, type DockerImage, type ImagePullProgress } from "@/domain/images";
import type { Host } from "@/domain/model";
import { BusyButton } from "@/shared/components/busy-button";
import { useBusyAction } from "@/shared/components/use-busy-action";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useImageCommands } from "./use-image-commands";

const SUGGESTIONS = ["redis:7.4-alpine", "postgres:17-alpine", "traefik/whoami:v1.11", "ghcr.io/example/app:1.2.0"];

/** Pull targets an allowed connected host and resolves saved credentials by registry host. */
export function PullDialog({ open, onOpenChange, hosts, images, initial }: { open: boolean; onOpenChange: (o: boolean) => void; hosts: Host[]; images: DockerImage[]; initial?: { hostId: string; reference: string } }) {
  const t = useTranslations("images.pullDialog");
  const cmd = useImageCommands();
  const firstOnline = hosts.find((h) => isHostAvailable(h))?.id ?? "";
  const [hostId, setHostId] = useState(initial?.hostId ?? firstOnline);
  const [registryId, setRegistryId] = useState("auto");
  const owner = `${hosts.map((host) => host.id).sort().join("|")}|${hostId}|${registryId}`;
  const feedback = useDialogCommand(open, owner);
  const change = (next: boolean) => feedback.change(next, onOpenChange);
  const [reference, setReference] = useState(initial?.reference ?? "");
  const [fail, setFail] = useState(false);
  const [progress, setProgress] = useState<{ owner: string; value: ImagePullProgress }>();
  const [touched, setTouched] = useState(false);
  const [seen, setSeen] = useState(open);
  if (open !== seen) {
    setSeen(open);
    if (open) {
      setHostId(initial?.hostId ?? firstOnline);
      setReference(initial?.reference ?? "");
      setFail(false);
      setProgress(undefined);
      setRegistryId("auto");
      setTouched(false);
    }
  }
  const registries = useRegistries(open);
  const selectedRegistry = registries.data?.find((r) => r.id === registryId && r.enabled);
  const effectiveReference = registryReference(reference, selectedRegistry && registryHost(selectedRegistry.url));
  const capabilities = useImageHostCapabilities("pullImage", hosts.map((h) => h.id));
  const capability = useCapability("pullImage", { kind: "host", id: hostId });
  const parsed = parseImageReference(effectiveReference);
  const known = parsed ? images.some((i) => i.hostId === hostId && i.tag !== null && sameImageReference(`${i.repository}:${i.tag}`, effectiveReference)) : false;
  const host = hosts.find((h) => h.id === hostId);
  const valid = !!parsed && isHostAvailable(host) && capability.available && (registryId === "auto" || !!selectedRegistry);

  // The request stays busy until the stream settles; detached attempts report their outcome by toast.
  const pull = useBusyAction(async () => {
    const current = feedback.checkpoint();
    const id = await feedback.run(() => cmd.pull({ hostId, reference: effectiveReference, onProgress: (value) => { if (current()) setProgress({ owner, value }); } }, fail), (result) => { if (result) toast.success(t("completed", { reference: effectiveReference, host: host?.name ?? hostId })); });
    if (id) change(false);
  });
  const submit = async () => {
    setTouched(true);
    if (!valid) return;
    await pull.run();
  };

  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogContent {...feedback.bindings} className={COMMAND_DIALOG_CONTENT} data-surface="pull-dialog">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        <form
          id="pull-image"
          className="flex min-w-0 flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <fieldset disabled={pull.busy} className="contents">
          <div className="flex min-w-0 flex-col gap-2">
            <Label htmlFor="pull-host">{t("host")}</Label>
            <Select disabled={pull.busy} value={hostId} onValueChange={setHostId}>
              <SelectTrigger id="pull-host" className="w-full min-w-0 [&>span]:min-w-0 [&>span]:truncate">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {hosts.map((h) => (
                    <SelectItem key={h.id} value={h.id} disabled={!isHostAvailable(h) || !capabilities.get(h.id)}>
                      {isHostAvailable(h) ? h.name : t("hostOffline", { name: h.name })}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <div className="flex min-w-0 flex-col gap-2"><Label htmlFor="pull-registry">{t("registry")}</Label><Select disabled={pull.busy} value={registryId} onValueChange={setRegistryId}><SelectTrigger id="pull-registry" className="w-full min-w-0 [&>span]:min-w-0 [&>span]:truncate"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="auto">{t("automaticRegistry")}</SelectItem>{registries.data?.filter((r) => r.enabled).map((r) => <SelectItem key={r.id} value={r.id}>{r.url}</SelectItem>)}</SelectContent></Select></div>
          <div className="min-w-0 [&_input]:min-w-0 [&_p]:break-words"><TextField id="pull-ref" label={t("reference")} value={reference} onChange={setReference} mono placeholder="redis:7.4-alpine" description={known ? t("known") : t("referenceHint")} error={(touched || reference.length > 2) && !parsed ? t("referenceInvalid") : undefined} /></div>
          {parsed && <p className="break-all text-sm" data-slot="effective-reference">{t("effectiveReference", { reference: `${parsed.repository}:${parsed.tag}` })}</p>}
          <div className="flex min-w-0 flex-wrap items-center gap-1.5 text-xs">
            <span className="text-muted-foreground">{t("suggestions")}</span>
            {SUGGESTIONS.map((s) => (
              <Button key={s} type="button" size="sm" variant="outline" className="h-auto min-h-6 min-w-0 max-w-full whitespace-normal break-all px-2 font-mono text-[11px]" onClick={() => setReference(s)}>
                {s}
              </Button>
            ))}
          </div>
          {import.meta.env.DEV && <div className="flex items-center gap-2">
            <Checkbox id="pull-fail" checked={fail} onCheckedChange={(v) => setFail(v === true)} />
            <Label htmlFor="pull-fail" className="text-sm font-normal text-muted-foreground">
              {t("fail")}
            </Label>
          </div>}
          </fieldset>
        </form>
        {progress?.owner === owner && <p role="status" className="min-w-0 break-words text-sm">{t(`progress.${progress.value.phase}`)}{progress.value.total ? ` · ${t("progressBytes", { current: formatBytes(progress.value.current ?? 0), total: formatBytes(progress.value.total) })}` : ""}</p>}
        <DialogCommandFooter feedback={feedback}>
          <Button type="button" variant="outline" onClick={() => change(false)}>
            {t("cancel")}
          </Button>
          <BusyButton type="submit" form="pull-image" disabled={!valid} busy={pull.busy} icon={<Download data-icon="inline-start" />}>
            {t("submit")}
          </BusyButton>
        </DialogCommandFooter>
      </DialogContent>
    </Dialog>
  );
}
