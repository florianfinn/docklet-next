// Source: docklet-next/docs/design/proxy-ui-concept.md
// Revision: 1a4a628bcb7569360c8126b781275c14cfc51074
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only proxy design concept with synthetic data; no upstream implementation copied.
import { useState } from "react";
import { Box, FileCode2, Radar, Server, Settings2 } from "lucide-react";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { Detail } from "./controls";
import { useProxy } from "./model";
import { ProxyConnectionDialog } from "./ProxyConnectionDialog";
import { LabSettings } from "./LabStatus";
import { useProxyText } from "./use-proxy-text";
export function ProxySetup({ instance, onConfigured }: { instance: string; onConfigured?: (id: string) => void }) {
  const t = useProxyText();
  const { connections, instances } = useProxy();
  const caddy = instances.find((p) => p.id === instance)?.kind === "caddy";
  const connection = connections[instance];
  const [editing, setEditing] = useState(false);
  return <>
    <Card className="max-w-4xl"><CardHeader><div className="flex flex-wrap items-start gap-4"><IconTile tone={connection ? "ok" : "accent"}>{connection ? <Server /> : <Radar />}</IconTile><div className="min-w-0 flex-1"><CardTitle>{connection ? instances.find((p) => p.id === instance)?.name : t("firstSetup")}</CardTitle><CardDescription className="mt-2">{t(connection ? "connectionConfiguredHint" : "firstSetupHint")}</CardDescription></div>{connection && <Button variant="outline" size="sm" onClick={() => setEditing(true)}><Settings2 data-icon="inline-start" />{t("editInstance")}</Button>}</div></CardHeader>
      <CardContent className="flex flex-col gap-5">{connection ? <><div className="flex flex-wrap items-center gap-3"><strong className="break-all text-lg">{connection.container}</strong><ToneBadge tone="cyan">{connection.host}</ToneBadge><ToneBadge tone="gray">{connection.image || t("manual")}</ToneBadge></div><dl><Detail label={t("configDirectory")}><code className="break-all text-xs">{connection.configPath}</code></Detail>{connection.apiUrl && <Detail label={t("apiUrl")}><code className="break-all text-xs">{connection.apiUrl}</code></Detail>}</dl></> : <div><Button onClick={() => setEditing(true)}><Radar data-icon="inline-start" />{t("chooseProxy")}</Button></div>}
        <div className="grid gap-4 border-t pt-5 sm:grid-cols-2">{([{ icon: FileCode2, labelKey: "fileProvider", hint: "readWrite" }, { icon: Box, labelKey: "dockerProvider", hint: "readCompose" }] as const).filter((p) => !caddy || p.labelKey !== "dockerProvider").map(({ icon: Icon, labelKey, hint }) => <div key={labelKey} className="flex gap-3"><Icon className="size-5 shrink-0 text-primary-text" /><div><p className="text-sm font-medium">{t(labelKey)}</p><p className="mt-1 text-xs text-muted-foreground">{t(hint)}</p></div></div>)}</div>
      </CardContent></Card>
    <LabSettings instance={instance} />
    {editing && <ProxyConnectionDialog instance={instance} onClose={() => setEditing(false)} onConfigured={(id) => onConfigured?.(id)} />}
  </>;
}
