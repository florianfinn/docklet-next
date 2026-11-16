// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { useEffect, useState } from "react";
import { useTranslations } from "use-intl";
import type { ProxyApi } from "@/data/http/proxy-api";
import { Button } from "@/shared/ui/button";
import { TextField } from "@/shared/components/form-field";
import { Choice, ErrorView, Reason } from "./controls";
import type { CreateInstance, Discovery, Instance } from "./types";

type Host = { id: string; name: string };
function Candidate({
  candidate: c,
  assigned,
  environment,
  busy,
  create,
}: {
  candidate: Discovery;
  assigned?: string;
  environment: string;
  busy: boolean;
  create: (input: CreateInstance) => void;
}) {
  const t = useTranslations("proxyContract.ui"),
    contract = useTranslations("proxyContract");
  const [source, setSource] = useState(c.sources?.[0]?.mountTarget ?? ""),
    [name, setName] = useState(c.identity.containerName);
  const [directory, setDirectory] = useState(""),
    [root, setRoot] = useState("");
  const selected = c.sources?.find(s => s.mountTarget === source);
  return (
    <article className="grid min-w-0 gap-4 rounded-xl border bg-card p-4">
      <div className="min-w-0 [overflow-wrap:anywhere]">
        <h3 className="font-semibold">{c.identity.containerName}</h3>
        <p className="text-sm">
          {c.image} {c.version}
        </p>
        {c.instanceId && <p>{t("assigned", { name: assigned ?? c.instanceId })}</p>}
      </div>
      <fieldset disabled={busy || !!c.instanceId} className="grid min-w-0 gap-4">
        <Choice
          id={`source-${c.identity.containerId}`}
          label={t("source")}
          value={source}
          onChange={setSource}
          placeholder={t("chooseSource")}
          options={(c.sources ?? []).map(s => ({
            value: s.mountTarget,
            label: `${s.mountTarget} · ${contract(`sources.${s.kind}`)}${s.readOnly ? ` · ${t("readOnly")}` : ""}`,
          }))}
        />
        {selected && <Reason code={selected.reason} />}
        <TextField id={`name-${c.identity.containerId}`} label={t("name")} value={name} onChange={setName} />
        <TextField
          id={`dir-${c.identity.containerId}`}
          label={t("routeDirectory")}
          value={directory}
          onChange={setDirectory}
        />
        {c.kind === "caddy" && (
          <TextField id={`root-${c.identity.containerId}`} label={t("rootFile")} value={root} onChange={setRoot} />
        )}
        <Button
          disabled={!source || !name.trim() || !directory.trim() || (c.kind === "caddy" && !root.trim())}
          onClick={() =>
            create({
              environmentId: environment,
              containerId: c.identity.containerId,
              kind: c.kind,
              mountTarget: source,
              name,
              routeDirectory: directory,
              ...(c.kind === "caddy" ? { rootFile: root } : {}),
            })
          }
        >
          {t("createInstance")}
        </Button>
      </fieldset>
    </article>
  );
}
export function Setup({
  api,
  hosts,
  instances = [],
  create,
  busy,
}: {
  api: ProxyApi;
  hosts: Host[];
  instances?: Instance[];
  create: (input: CreateInstance) => void;
  busy: boolean;
}) {
  const t = useTranslations("proxyContract.ui");
  const [environment, setEnvironment] = useState(""),
    [revision, refresh] = useState(0);
  const [state, setState] = useState<{ rows?: Discovery[]; error?: unknown }>({});
  useEffect(() => {
    if (!environment) return;
    let active = true;
    void api.discover(environment).then(
      rows => {
        if (active) setState({ rows });
      },
      error => {
        if (active) setState({ error });
      },
    );
    return () => {
      active = false;
    };
  }, [api, environment, revision]);
  return (
    <section className="grid min-w-0 gap-4" aria-label={t("setup")}>
      <h2 className="text-xl font-semibold">{t("setup")}</h2>
      <p className="text-sm text-muted-foreground">{t("noInstances")}</p>
      <Choice
        id="proxy-host"
        label={t("host")}
        placeholder={t("chooseHost")}
        value={environment}
        onChange={v => {
          setState({});
          setEnvironment(v);
        }}
        options={hosts.map(h => ({ value: h.id, label: h.name }))}
      />
      <Button
        variant="outline"
        disabled={!environment || busy}
        onClick={() => {
          setState({});
          refresh(n => n + 1);
        }}
      >
        {t("discover")}
      </Button>
      {state.error !== undefined && <ErrorView error={state.error} />}
      {environment && !state.rows && state.error === undefined && <p role="status">{t("loading")}</p>}
      <div className="grid min-w-0 gap-4 lg:grid-cols-2">
        {state.rows?.map(c => (
          <Candidate
            key={`${environment}:${c.identity.containerId}`}
            candidate={c}
            assigned={instances.find(i => i.id === c.instanceId)?.name}
            environment={environment}
            busy={busy}
            create={create}
          />
        ))}
      </div>
    </section>
  );
}
