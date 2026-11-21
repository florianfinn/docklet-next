// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { Detail } from "./presentation";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { Server } from "lucide-react";
import { useEffect, useState } from "react";
import { PROXY_REASON_KEYS } from "@/data/wire-catalogue-proxy";
import { useTranslations } from "use-intl";
import type { ProxyApi } from "@/data/http/proxy-api";
import { Button } from "@/shared/ui/button";
import { TextField } from "@/shared/components/form-field";
import { Choice, ErrorView, Reason } from "./controls";
import type { CreateInstance, Discovery, Instance } from "./types";

type Source = NonNullable<Discovery["sources"]>[number];
// Sources without eligibility (older agents) stay selectable with the previous free fields.
const selectable = (s: Source) => s.eligibility?.selectable !== false;
const fields = (s?: Source) => ({ directory: s?.eligibility?.routeDirectory?.value ?? "", root: s?.eligibility?.rootFile ?? "" });

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
    contract = useTranslations("proxyContract"),
    all = useTranslations();
  const sources = c.sources ?? [];
  const initial = sources.find(selectable);
  const [source, setSource] = useState(initial?.mountTarget ?? ""),
    [name, setName] = useState(c.identity.containerName);
  const [directory, setDirectory] = useState(fields(initial).directory),
    [root, setRoot] = useState(fields(initial).root);
  const selected = sources.find(s => s.mountTarget === source);
  const fixed = selected?.eligibility?.routeDirectory?.fixed === true;
  const reasonText = (code?: string) => {
    const key = code ? PROXY_REASON_KEYS[code as keyof typeof PROXY_REASON_KEYS] : undefined;
    return key ? all(key) : code ? t("unknownReason", { code }) : "";
  };
  const choose = (target: string) => {
    setSource(target);
    const next = fields(sources.find(s => s.mountTarget === target));
    setDirectory(next.directory);
    setRoot(next.root);
  };
  return (
    <article className="grid min-w-0 gap-4 rounded-xl border bg-card p-4">
      <div className="min-w-0 [overflow-wrap:anywhere]">
        <div className="flex items-center gap-3"><IconTile tone="accent"><Server /></IconTile><h3 className="font-semibold">{c.identity.containerName}</h3><ToneBadge tone="cyan">{contract(`kinds.${c.kind}`)}</ToneBadge></div>
        <dl><Detail label={t("container")}>{c.image} {c.version}</Detail></dl>
        {c.instanceId && <p>{t("assigned", { name: assigned ?? c.instanceId })}</p>}
      </div>
      <fieldset disabled={busy || !!c.instanceId} className="grid min-w-0 gap-4">
        {!initial ? (
          <p role="status" className="text-sm">
            {t("noEligibleSource")}
          </p>
        ) : (
          <Choice
            id={`source-${c.identity.containerId}`}
            label={t("source")}
            value={source}
            onChange={choose}
            placeholder={t("chooseSource")}
            options={sources.map(s => ({
              value: s.mountTarget,
              disabled: !selectable(s),
              // Host or project path first; the container path is secondary.
              label: [
                `${s.kind === "project" && s.projectPath ? s.projectPath : s.canonical} → ${s.mountTarget}`,
                contract(`sources.${s.kind}`),
                ...(s.readOnly ? [t("readOnly")] : []),
                ...(selectable(s) ? [] : [reasonText(s.eligibility?.reason)]),
              ].join(" · "),
            }))}
          />
        )}
        {sources.length > 0 && !initial && sources.map(s => (
          <p key={s.mountTarget} className="text-sm text-muted-foreground [overflow-wrap:anywhere]">
            {s.mountTarget} · {reasonText(s.eligibility?.reason)}
          </p>
        ))}
        {selected && <Reason code={selected.reason} />}
        <TextField id={`name-${c.identity.containerId}`} label={t("name")} value={name} onChange={setName} />
        <TextField
          id={`dir-${c.identity.containerId}`}
          label={t("routeDirectory")}
          value={fixed && directory === "." ? t("sourceRoot") : directory}
          onChange={setDirectory}
          disabled={fixed}
        />
        <p className="text-sm text-muted-foreground">{t(fixed ? "routeDirectoryFixedHint" : selected?.eligibility ? "routeDirectoryHint" : "routeDirectoryLegacyHint")}</p>
        {c.kind === "caddy" && (
          <>
            <TextField id={`root-${c.identity.containerId}`} label={t("rootFile")} value={root} onChange={setRoot} />
            <p className="text-sm text-muted-foreground">{t("rootFileHint")}</p>
          </>
        )}
        <Button
          className="justify-self-start"
          disabled={!selected || !selectable(selected) || !name.trim() || !directory.trim() || (c.kind === "caddy" && !root.trim())}
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
        className="justify-self-start"
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
