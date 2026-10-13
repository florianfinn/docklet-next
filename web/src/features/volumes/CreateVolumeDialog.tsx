import { useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import type { Host } from "@/domain/model";
import { volumeName } from "@/domain/validation";
import type { Volume } from "@/domain/volumes";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Spinner } from "@/shared/ui/spinner";
import { Textarea } from "@/shared/ui/textarea";

import { useVolumeCommands } from "./use-volume-commands";

function parseLabels(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const [k, ...v] = line.split("=");
    if (k?.trim()) out[k.trim()] = v.join("=").trim();
  }
  return out;
}

export function CreateVolumeDialog({ open, onOpenChange, hosts, volumes, defaultHostId }: { open: boolean; onOpenChange: (o: boolean) => void; hosts: Host[]; volumes: Volume[]; defaultHostId?: string }) {
  const t = useTranslations("volumes.createDialog");
  const tv = useTranslations("hosts.validation");
  const cmd = useVolumeCommands();
  const navigate = useNavigate();
  const online = hosts.filter((h) => h.status === "online");
  const [name, setName] = useState("");
  const [hostId, setHostId] = useState(defaultHostId ?? online[0]?.id ?? "");
  const [labels, setLabels] = useState("");
  const [tried, setTried] = useState(false);
  const [busy, setBusy] = useState(false);
  const taken = volumes.filter((v) => v.hostId === hostId && v.kind === "volume").map((v) => v.name);
  const error = volumeName(name, taken);
  const message = error === "volumeName" ? t("nameHint") : error ? tv(error as "required") : undefined;

  const reset = () => {
    setName("");
    setLabels("");
    setTried(false);
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        onOpenChange(o);
        if (!o) reset();
      }}
    >
      <DialogContent>
        <form
          className="flex flex-col gap-4"
          onSubmit={async (e) => {
            e.preventDefault();
            setTried(true);
            if (error || !hostId) return;
            setBusy(true);
            const created = await cmd.create({ name: name.trim(), hostId, labels: parseLabels(labels) });
            setBusy(false);
            if (!created) return;
            toast.success(t("created", { name: created.name }));
            onOpenChange(false);
            reset();
            navigate(`/volumes/${created.id}`);
          }}
        >
          <DialogHeader>
            <DialogTitle>{t("title")}</DialogTitle>
            <DialogDescription>{t("description")}</DialogDescription>
          </DialogHeader>
          <TextField id="vol-name" label={t("name")} value={name} onChange={setName} description={t("nameHint")} error={tried ? message : undefined} mono placeholder="werkstatt_daten" />
          <Field>
            <FieldLabel htmlFor="vol-host">{t("host")}</FieldLabel>
            <Select value={hostId} onValueChange={setHostId}>
              <SelectTrigger id="vol-host" className="w-full">
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
          <Field>
            <FieldLabel>{t("driver")}</FieldLabel>
            <p className="font-mono text-sm text-muted-foreground">{t("driverValue")}</p>
          </Field>
          <Field>
            <FieldLabel htmlFor="vol-labels">{t("labels")}</FieldLabel>
            <Textarea id="vol-labels" value={labels} onChange={(e) => setLabels(e.target.value)} className="min-h-16 font-mono text-[13px]" placeholder="docklet.owner=werkstatt" />
            <FieldDescription>{t("labelsHint")}</FieldDescription>
          </Field>
          <DialogFooter>
            <Button type="submit" disabled={busy || !online.length}>
              {busy && <Spinner data-icon="inline-start" />}
              {t("submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
