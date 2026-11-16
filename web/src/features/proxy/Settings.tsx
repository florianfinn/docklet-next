// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { Button } from "@/shared/ui/button";
import { TextField } from "@/shared/components/form-field";
import { ConfirmCheck, EligibilityView, ProxyDialog } from "./controls";
import type { Instance, Inventory, Settings as InstanceSettings } from "./types";

export function Settings({
  instance,
  inventory,
  busy,
  save,
  remove,
}: {
  instance: Instance;
  /** Absent when the instance cannot be read (e.g. its container was recreated): removal stays available. */
  inventory?: Inventory;
  busy: boolean;
  save: (input: InstanceSettings) => void;
  remove: () => void;
}) {
  const t = useTranslations("proxyContract.ui"),
    contract = useTranslations("proxyContract");
  const [name, setName] = useState(instance.name),
    [directory, setDirectory] = useState(instance.routeDirectory),
    [root, setRoot] = useState(instance.rootFile ?? "");
  const [removing, setRemoving] = useState(false),
    [confirmed, setConfirmed] = useState(false);
  const facts = !inventory
    ? []
    : instance.kind === "traefik"
      ? [
          [t("entryPoints"), inventory.entryPoints?.map(ep => `${ep.name} · ${ep.address}`).join(", ")],
          [t("resolvers"), inventory.resolvers?.join(", ")],
          [t("providers"), inventory.providers?.join(", ")],
          [t("plugins"), inventory.plugins?.join(", ")],
          [t("fileProvider"), inventory.fileProvider],
          [t("watch"), t(inventory.watch ? "yes" : "no")],
          [t("throttle"), String(inventory.throttleMillis)],
        ]
      : [
          [t("rootFile"), inventory.rootFile],
          [t("imports"), inventory.imports?.join("\n")],
          [t("adminAddress"), inventory.adminAddress],
          [t("adminOff"), t(inventory.adminOff ? "yes" : "no")],
          [t("resume"), t(inventory.resume ? "yes" : "no")],
          [t("persistConfig"), t(inventory.persistConfig ? "yes" : "no")],
        ];
  return (
    <section className="grid min-w-0 gap-6">
      <div className="grid gap-3 rounded-xl border p-4">
        <h2 className="font-semibold">{t("status")}</h2>
        {inventory && <EligibilityView value={inventory.eligibility} />}
        <p className="break-all text-sm">
          {t("authority")} · {t(`authorities.${instance.authority}`)}
        </p>
        <p className="break-all text-sm">
          {contract(`sources.${instance.source.kind}`)} · {instance.source.mountTarget}
        </p>
        {facts.length > 0 && (
          <dl className="grid min-w-0 gap-4 sm:grid-cols-2">
            {facts.map(([label, value]) => (
              <div key={label} className="min-w-0">
                <dt className="text-xs text-muted-foreground">{label}</dt>
                <dd className="whitespace-pre-wrap break-all text-sm">{value || "—"}</dd>
              </div>
            ))}
          </dl>
        )}
      </div>
      <form
        className="grid min-w-0 gap-4 rounded-xl border p-4"
        onSubmit={e => {
          e.preventDefault();
          if (!busy && name.trim() && directory.trim())
            save({ name, routeDirectory: directory, ...(instance.kind === "caddy" ? { rootFile: root } : {}) });
        }}
      >
        <fieldset disabled={busy} className="grid min-w-0 gap-4">
          <TextField id="instance-name" label={t("name")} value={name} onChange={setName} />
          <TextField id="instance-directory" label={t("routeDirectory")} value={directory} onChange={setDirectory} />
          {instance.kind === "caddy" && (
            <TextField id="instance-root" label={t("rootFile")} value={root} onChange={setRoot} />
          )}
        </fieldset>
        <Button disabled={busy || !name.trim() || !directory.trim()}>{t("save")}</Button>
      </form>
      <Button variant="destructive" disabled={busy} onClick={() => setRemoving(true)}>
        {t("remove")}
      </Button>
      {removing && (
        <ProxyDialog title={t("remove")} description={t("removeHint")} onClose={() => setRemoving(false)} busy={busy}>
          <ConfirmCheck checked={confirmed} onChange={setConfirmed}>
            {t("confirm")}
          </ConfirmCheck>
          <Button
            variant="destructive"
            disabled={!confirmed || busy}
            onClick={() => {
              if (confirmed && !busy) remove();
            }}
          >
            {t("remove")}
          </Button>
        </ProxyDialog>
      )}
    </section>
  );
}
