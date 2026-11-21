// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { Detail } from "./presentation";
import { ToneBadge } from "@/shared/components/tone";
import { ResolverDialog } from "./ResolverDialog";
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
  const [resolverOpen,setResolverOpen]=useState(false);
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
      <div className="grid gap-3 rounded-xl border bg-card p-5">
        <h2 className="font-semibold">{t("status")}</h2>
        {inventory && <EligibilityView value={inventory.eligibility} />}
        <dl><Detail label={t("authority")}><ToneBadge tone="cyan">{t(`authorities.${instance.authority}`)}</ToneBadge></Detail><Detail label={t("source")}>{contract(`sources.${instance.source.kind}`)} · {instance.source.mountTarget}</Detail></dl>
        {facts.length > 0 && (
          <dl className="min-w-0">
            {facts.map(([label, value]) => (
              <Detail key={label} label={label!}>{value || "—"}</Detail>
            ))}
          </dl>
        )}
      </div>
      {instance.kind === "traefik" && inventory && (
        <section className="grid gap-3 rounded-xl border bg-card p-5">
          <h2 className="font-semibold">{t("resolvers")}</h2>
          <ul className="grid gap-2">
            {(inventory.resolverSummaries ?? []).map(resolver => <li key={resolver.name} className="break-all text-sm">
              <strong>{resolver.name}</strong> · {resolver.challenge.kind.toUpperCase()}-01 · {resolver.storage}
              {resolver.variables?.map(variable => <span key={variable.name}> · {variable.name}: {t(variable.configured ? "yes" : "no")}</span>)}
            </li>)}
          </ul>
          <Button className="justify-self-start" variant="outline" disabled={busy || !inventory.eligibility.writable} onClick={() => setResolverOpen(true)}>{t("resolverCreate.create")}</Button>
        </section>
      )}
      {resolverOpen && inventory && <ResolverDialog instance={instance} inventory={inventory} close={() => setResolverOpen(false)} />}
      <form
        className="grid min-w-0 gap-4 rounded-xl border bg-card p-5"
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
        <Button className="justify-self-start" disabled={busy || !name.trim() || !directory.trim()}>{t("save")}</Button>
      </form>
      <Button className="justify-self-start" variant="destructive" disabled={busy} onClick={() => setRemoving(true)}>
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
