// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original scoped origin assignment, confirmation and detection reset UI.

import { ComposeContainerHint } from "./ComposeContainerHint";
import { Lock } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";
import { useCapability } from "@/data/use-capability";
import type { ContainerInfo } from "@/domain/model";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { Alert, AlertDescription, AlertTitle } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

export function ContainerProvenance({ c }: { c: ContainerInfo }) {
  return <>{c.origin === "compose" && <ComposeContainerHint c={c} />}{(c.origin !== "compose" || c.originSource === "unconfirmed") && <OriginControls key={c.id} c={c} />}</>;
}

function OriginControls({ c }: { c: ContainerInfo }) {
  const t = useTranslations("containers.assignment");
  const to = useTranslations("containers.origin");
  const cmd = useContainerCommands();
  const capability = useCapability("setContainerOrigin", { kind: "container", id: c.id });
  const [open, setOpen] = useState(false);
  const [target, setTarget] = useState<"unraid" | "standalone">("unraid");
  const [pending, setPending] = useState(false);
  const partialCompose = Object.keys(c.dockerLabels ?? {}).some((key) => key.startsWith("com.docker.compose."));
  const unconfirmed = c.originSource === "unconfirmed";
  const manual = c.originSource === "manual";
  const canAssign = c.origin === "unknown" || manual;
  if (c.origin !== "unknown" && c.origin !== "unraid" && !manual && !unconfirmed) return null;
  const submit = async (reset = false) => {
    setPending(true);
    try { if (await cmd.setOrigin(c.id, reset ? "unknown" : target)) setOpen(false); }
    finally { setPending(false); }
  };
  return <>
    <Alert>
      <Lock />
      <AlertTitle>{unconfirmed ? t("unconfirmed") : manual ? t("manual") : to(`${c.origin}Title`)}</AlertTitle>
      <AlertDescription className="flex flex-col items-start gap-2">
        <p>{unconfirmed ? t("unconfirmedHint") : to(`${c.origin}Body`)}</p>
        {(c.origin === "unknown" || manual || unconfirmed) && <Button size="sm" variant="outline" disabled={!capability.available || !!c.inventory?.stale} title={capability.reason} onClick={() => setOpen(true)}>{unconfirmed ? t(canAssign ? "confirm" : "review") : t("title")}</Button>}
      </AlertDescription>
    </Alert>
    <Dialog open={open} onOpenChange={(next) => { if (!pending) setOpen(next); }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{t("title")}</DialogTitle><DialogDescription>{t("hint")}</DialogDescription></DialogHeader>
        {canAssign ? <div className="flex flex-col gap-2">
          <Label htmlFor="container-origin">{t("target")}</Label>
          <Select value={target} onValueChange={(value) => setTarget(value as "unraid" | "standalone")} disabled={pending}>
            <SelectTrigger id="container-origin"><SelectValue /></SelectTrigger>
            <SelectContent>
              <SelectItem value="unraid">{to("unraid")}</SelectItem>
              {!partialCompose && <SelectItem value="standalone">{to("standalone")}</SelectItem>}
            </SelectContent>
          </Select>
          {partialCompose && <p className="text-sm text-muted-foreground">{t("partialCompose")}</p>}
        </div> : <p className="text-sm text-muted-foreground">{t("detectionOnly")}</p>}
        <DialogFooter>
          {(manual || unconfirmed) && <Button variant="outline" disabled={pending || !capability.available} onClick={() => void submit(true)}>{t("reset")}</Button>}
          {canAssign && <Button disabled={pending || !capability.available} onClick={() => void submit()}>{unconfirmed ? t("confirm") : t("save")}</Button>}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </>;
}
