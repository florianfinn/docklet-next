// Source: web/src/features/design-lab/proxy/RouteFields.tsx
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Real same-host running inventory; deliberate lossless backend edits, TCP ports and connectivity hints.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { useContainers } from "@/data/queries";
import { ChoiceCards, TextField } from "@/shared/components/form-field";
import { Choice } from "./controls";
import { FieldHelp } from "./presentation";
import { containerSuggestions, offNetwork, tcpPorts } from "./suggestions";
import type { backendParts } from "./model";
import type { Instance } from "./types";

type Backend = ReturnType<typeof backendParts>;
export function DestinationFields({ instance, backend, update }: { instance: Instance; backend: Backend; update: (f: (b: Backend) => Backend) => void }) {
  const t = useTranslations("proxyContract.ui");
  const containers = useContainers();
  const rows = containers.data ?? [];
  const candidates = containerSuggestions(instance, rows);
  const [kind, setKind] = useState("external"), [picked, setPicked] = useState("");
  const selected = candidates.find(c => c.id === picked);
  const ports = selected ? tcpPorts(selected) : [];
  const pick = (id: string) => {
    const c = candidates.find(row => row.id === id);
    if (!c) return;
    setPicked(id);
    const ports = tcpPorts(c), port = ports.length === 1 ? ports[0]! : "";
    update(b => ({ ...b, host: c.name, port, colon: port !== "" }));
  };
  return <>
    <FieldHelp label={t("destination")} help={t("help.destination")}>
      <ChoiceCards label={t("destination")} value={kind} onChange={setKind} options={[
        { value: "container", title: t("containerDestination"), description: t("containerDestinationHint") },
        { value: "external", title: t("externalDestination"), description: t("externalDestinationHint") },
      ]} />
    </FieldHelp>
    {kind === "container" && <>
      <FieldHelp label={t("chooseContainer")} help={t("help.container")}>
        <Choice id="destination-container" label={t("chooseContainer")} value={selected?.id ?? ""} placeholder={t("chooseContainer")} onChange={pick}
          options={candidates.map(c => ({ value: c.id, label: [c.stackName, c.name, offNetwork(instance, c, rows) ? t("offNetwork") : ""].filter(Boolean).join(" · ") }))} />
      </FieldHelp>
      {selected && offNetwork(instance, selected, rows) && <p role="status" className="text-sm text-warning">{t("offNetwork")}</p>}
      {picked && !selected && !containers.isPending && <p role="status" className="text-sm text-warning">{t("containerGone")}</p>}
      {ports.length > 1 && <FieldHelp label={t("containerPort")} help={t("help.port")}>
        <Choice id="container-port" label={t("containerPort")} value={ports.includes(backend.port) ? backend.port : ""} placeholder={t("choosePort")} options={ports.map(p => ({ value: p, label: p }))}
          onChange={port => update(b => ({ ...b, port, colon: port !== "" }))} />
      </FieldHelp>}
    </>}
    <div className="grid min-w-0 gap-3 sm:grid-cols-[1fr_2fr_1fr]">
      <FieldHelp label={t("scheme")} help={t("help.scheme")}><Choice id="backend-scheme" label={t("scheme")} value={backend.scheme} onChange={scheme => update(b => ({ ...b, scheme }))}
        options={[...new Set(["http", "https", ...(backend.scheme ? [backend.scheme] : [])])].map(v => ({ value: v, label: v }))} /></FieldHelp>
      <FieldHelp label={t("backendHost")} help={t("help.backendHost")}><TextField id="backend-host" label={t("backendHost")} value={backend.host} onChange={host => update(b => ({ ...b, host }))} /></FieldHelp>
      <FieldHelp label={t("port")} help={t("help.port")}><TextField id="backend-port" label={t("port")} value={backend.port} onChange={port => update(b => ({ ...b, port, colon: port !== "" }))} /></FieldHelp>
    </div>
  </>;
}
