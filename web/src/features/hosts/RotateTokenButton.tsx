// Source: docklet-next/docs/design/host-onboarding.md
// Revision: 81439fab6a8ec5232161dc933fbd41bec41885fa
// Retrieved: 2026-10-17
// License: Apache-2.0
// Deviations: Original one-time token rotation dialog.

import { KeyRound } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { useCapability } from "@/data/use-capability";
import type { Host } from "@/domain/model";
import { CodeBlock } from "@/shared/components/code-block";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { useHostCommands } from "./use-host-commands";

/** The credential stays in this open dialog and never enters the host query cache. */
export function RotateTokenButton({ host }: { host: Host }) {
  const t = useTranslations("hosts.rotation");
  const capability = useCapability("issueAgentToken");
  const commands = useHostCommands();
  const [confirm, setConfirm] = useState(false);
  const [issued, setIssued] = useState<string | null>(null);
  if (!capability.available || host.id === "0") return null;
  return <>
    <Button size="sm" variant="outline" onClick={() => setConfirm(true)}><KeyRound data-icon="inline-start" />{t("action")}</Button>
    <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={t("title", { name: host.name })} description={t("body")} confirmLabel={t("action")} destructive onConfirm={async () => {
      const token = await commands.token(host.id);
      if (!token) return false;
      setIssued(token);
      return true;
    }} />
    <Dialog open={issued !== null} onOpenChange={(open) => { if (!open) setIssued(null); }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{t("result", { name: host.name })}</DialogTitle><DialogDescription>{t("hint")}</DialogDescription></DialogHeader>
        {issued && <CodeBlock code={issued} label={t("result", { name: host.name })} copyLabel={t("copy")} />}
      </DialogContent>
    </Dialog>
  </>;
}
