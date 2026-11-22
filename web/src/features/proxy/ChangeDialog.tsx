// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation. M35 #559: the named action confirms; risk acknowledgements stay.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { Button } from "@/shared/ui/button";
import { Textarea } from "@/shared/ui/textarea";
import { Label } from "@/shared/ui/label";
import { AcknowledgementFields, RefusalView, ProxyDialog } from "./controls";
import { acknowledgementsReady, emptyAcknowledgements } from "./model";
import type { Acknowledgements, Change, Inventory, Route } from "./types";

export type ChangeAction = {
  kind: "delete-route" | "insert-import" | "restore-owned" | "edit-service";
  target: string;
  route?: Route;
};
export function ChangeDialog({
  action,
  inventory,
  busy,
  caddy = false,
  refusal,
  submit,
  close,
}: {
  action: ChangeAction;
  inventory: Inventory;
  busy: boolean;
  caddy?: boolean;
  refusal?: unknown;
  submit: (target: string, change: Change, ack: Acknowledgements) => void;
  close: () => void;
}) {
  const t = useTranslations("proxyContract.ui"),
    [ack, setAck] = useState(emptyAcknowledgements);
  const [upstreams, setUpstreams] = useState(action.route?.upstreams?.join("\n") ?? "");
  const title =
    action.kind === "edit-service"
      ? "editService"
      : action.kind === "delete-route"
        ? "delete"
        : action.kind === "insert-import"
          ? "insertImport"
          : "restore";
  const hint =
    action.kind === "edit-service"
      ? "sharedServiceHint"
      : action.kind === "delete-route"
        ? caddy
          ? "deleteHintCaddy"
          : "deleteHint"
        : action.kind === "insert-import"
          ? "insertHint"
          : "restoreHint";
  // The named button is the confirmation; only risk acknowledgements gate it.
  const ready = acknowledgementsReady(inventory, [], ack) && (action.kind !== "edit-service" || !!upstreams.trim());
  return (
    <ProxyDialog title={t(title)} description={t(hint)} onClose={close} busy={busy}>
      <p className="break-all font-mono text-xs">{action.target}</p>
      {action.kind === "insert-import" && <pre className="whitespace-pre-wrap break-all">{inventory.importLine}</pre>}
      {action.kind === "edit-service" && (
        <>
          <h3 className="font-medium">{t("dependants")}</h3>
          <ul className="list-inside list-disc break-all">
            {action.route?.dependants?.map(r => (
              <li key={r}>{r}</li>
            ))}
          </ul>
          <Label htmlFor="service-upstreams">{t("upstreams")}</Label>
          <Textarea id="service-upstreams" value={upstreams} onChange={e => setUpstreams(e.target.value)} />
        </>
      )}
      <AcknowledgementFields inventory={inventory} value={ack} onChange={setAck} />
      <Button
        disabled={!ready || busy}
        onClick={() => {
          if (ready && !busy)
            submit(
              action.target,
              {
                kind: action.kind,
                ...(action.route ? { routeId: action.route.id } : {}),
                ...(action.kind === "edit-service"
                  ? {
                      service: action.route?.service,
                      // Lines are sent as written; untouched URLs keep their bytes.
                      upstreams: upstreams.split("\n").filter(s => s !== ""),
                    }
                  : {}),
              },
              ack,
            );
        }}
      >
        {t(action.kind === "edit-service" ? "saveService" : title)}
      </Button>
      <RefusalView refusal={refusal} />
    </ProxyDialog>
  );
}
