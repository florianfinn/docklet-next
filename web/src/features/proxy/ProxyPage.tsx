// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { usePrincipal } from "@/data/use-principal";
import { useHosts } from "@/data/queries";
import { identityLifetime } from "@/data/identity-lifetime";
import { createProxyApi } from "@/data/http/proxy-api";
import { Button } from "@/shared/ui/button";
import { Choice, ErrorView } from "./controls";
import { InstanceWorkspace } from "./InstanceWorkspace";
import { Setup } from "./Setup";
import type { CreateInstance, Instance } from "./types";

function Manager() {
  const t = useTranslations("proxyContract.ui"),
    hosts = useHosts();
  const adapter = useAdapter(),
    lifetime = identityLifetime(adapter);
  const active = useRef(false),
    lock = useRef(false);
  const [connection] = useState(() => {
    let controller = new AbortController();
    return {
      api: createProxyApi(() => controller.signal),
      start: () => {
        controller = new AbortController();
      },
      stop: () => controller.abort(),
    };
  });
  const api = connection.api;
  const [instances, setInstances] = useState<Instance[]>(),
    [selected, setSelected] = useState(""),
    [setup, setSetup] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<unknown>();
  useEffect(() => {
    let live = true;
    active.current = true;
    connection.start();
    const stop = lifetime.listen(() => {
      connection.stop();
    });
    void api.instances().then(
      rows => {
        if (live && active.current) {
          setError(undefined);
          setInstances(rows);
          setSelected(rows[0]?.id ?? "");
        }
      },
      e => {
        if (live && active.current) setError(e);
      },
    );
    return () => {
      live = false;
      active.current = false;
      connection.stop();
      stop();
    };
  }, [api, lifetime, connection]);
  const instance = instances?.find(i => i.id === selected);
  const create = async (input: CreateInstance) => {
    if (lock.current) return;
    lock.current = true;
    setBusy(true);
    setError(undefined);
    try {
      const i = await api.create(input);
      if (active.current) {
        setInstances(old => [...(old ?? []), i]);
        setSelected(i.id);
        setSetup(false);
      }
    } catch (e) {
      if (active.current) setError(e);
    } finally {
      lock.current = false;
      if (active.current) setBusy(false);
    }
  };
  return (
    <div className="[&_button]:max-w-full [&_button]:whitespace-normal mx-auto grid w-full min-w-0 max-w-6xl gap-6 p-4 sm:p-6">
      <header>
        <h1 className="text-2xl font-semibold">{t("title")}</h1>
        <p className="mt-1 text-sm text-muted-foreground">{t("intro")}</p>
      </header>
      {error !== undefined && <ErrorView error={error} />}
      {hosts.isError && <ErrorView error={hosts.error} />}
      {instances && (
        <>
          <div className="grid min-w-0 gap-3 rounded-xl border bg-card p-4 sm:grid-cols-[1fr_auto]">
            <Choice
              id="proxy-instance"
              label={t("instances")}
              value={selected}
              placeholder={t("instance")}
              onChange={v => {
                setSelected(v);
                setSetup(false);
              }}
              options={instances.map(i => ({
                value: i.id,
                label: `${i.name} · ${hosts.data?.find(h => h.id === i.environmentId)?.name ?? i.environmentId} · ${i.identity.containerName}`,
              }))}
            />
            <Button variant="outline" className="self-end" onClick={() => setSetup(true)}>
              {t("newInstance")}
            </Button>
          </div>
          {instance && !setup ? (
            <>
              <div className="flex min-w-0 flex-wrap gap-x-5 gap-y-2 text-sm">
                <span className="break-all">
                  {t("instance")} · {instance.name}
                </span>
                <span className="break-all">
                  {t("host")} · {hosts.data?.find(h => h.id === instance.environmentId)?.name ?? instance.environmentId}
                </span>
                <span className="break-all">
                  {t("container")} · {instance.identity.containerName}
                </span>
              </div>
              <InstanceWorkspace
                key={JSON.stringify(instance)}
                instance={instance}
                api={api}
                updated={i => setInstances(old => old?.map(row => (row.id === i.id ? i : row)))}
                removed={() => {
                  setInstances(old => old?.filter(row => row.id !== instance.id));
                  setSelected("");
                }}
              />
            </>
          ) : (
            <>
              <h2 className="text-sm font-medium">{t("candidates")}</h2>
              <Setup
                key={instances.length}
                api={api}
                hosts={hosts.data ?? []}
                instances={instances ?? []}
                create={input => {
                  void create(input);
                }}
                busy={busy}
              />
              {instance && (
                <Button variant="ghost" onClick={() => setSetup(false)}>
                  {t("back")}
                </Button>
              )}
            </>
          )}
        </>
      )}
      {!instances && error === undefined && <p role="status">{t("loading")}</p>}
      {!instances && error !== undefined && (
        <Button
          onClick={() => {
            void api.instances().then(
              rows => {
                if (active.current) {
                  setError(undefined);
                  setInstances(rows);
                  setSelected(rows[0]?.id ?? "");
                }
              },
              e => {
                if (active.current) setError(e);
              },
            );
          }}
        >
          {t("refresh")}
        </Button>
      )}
    </div>
  );
}
export default function ProxyPage() {
  const adapter = useAdapter(),
    principal = usePrincipal(),
    lifetime = identityLifetime(adapter);
  const version = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  if (!lifetime.active() || principal.data?.isAdmin === false) return null;
  return <Manager key={`${principal.data?.id ?? ""}:${version}`} />;
}
