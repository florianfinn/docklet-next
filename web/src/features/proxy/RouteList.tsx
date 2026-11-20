// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { Link } from "react-router";
import { MoreHorizontal } from "lucide-react";
import { useTranslations } from "use-intl";
import { inventoryId } from "@/data/http/inventory-mapping";
import { Button } from "@/shared/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";
import { PROXY_REFERENCE_KEYS } from "@/data/wire-catalogue-proxy";
import { EligibilityView } from "./controls";
import { CertificateCheck } from "./CertificateCheck";
import { canEdit } from "./model";
import type { Instance, Route } from "./types";
import type { ChangeAction } from "./ChangeDialog";

export function RouteList({
  instance,
  routes,
  hashes,
  writable,
  edit,
  action,
}: {
  instance: Instance;
  routes: Route[];
  hashes?: Record<string, string>;
  writable: boolean;
  edit: (r: Route) => void;
  action: (a: ChangeAction) => void;
}) {
  const t = useTranslations(),
    ui = useTranslations("proxyContract.ui");
  return (
    <div className="min-w-0 rounded-xl border bg-card">
      <div className="hidden grid-cols-[1fr_2fr_1fr_auto] gap-4 border-b p-4 text-xs font-medium text-muted-foreground md:grid">
        <span>{ui("name")}</span>
        <span>{ui("domain")}</span>
        <span>{ui("service")}</span>
        <span>{ui("actionsHeader")}</span>
      </div>
      {routes.length === 0 && <p className="p-5 text-sm">{ui("empty")}</p>}
      {routes.map(r => (
        <article
          key={r.id}
          aria-label={r.name}
          className="grid min-w-0 gap-3 border-b p-4 last:border-b-0 md:grid-cols-[1fr_2fr_1fr_auto]"
        >
          <div className="min-w-0 [overflow-wrap:anywhere]">
            <h3 className="font-medium">{r.name}</h3>
            <p className="text-xs text-muted-foreground">{ui(`classes.${r.class}`)}</p>
            <p className="mt-1 break-all font-mono text-xs">{r.file}</p>
          </div>
          <div className="min-w-0 space-y-2 [overflow-wrap:anywhere]">
            <p className="font-mono text-xs">{r.rule}</p>
            <EligibilityView value={r.eligibility} />
            <ul className="text-xs" aria-label={ui("references")}>
              {(r.references ?? []).map((ref, n) => (
                <li key={`${ref.name}:${n}`}>
                  {ref.name} · {t(PROXY_REFERENCE_KEYS[ref.kind])}
                  {ref.file ? ` · ${ref.file}` : ""}
                </li>
              ))}
            </ul>
            {r.class === "label-read-only" && r.projectId && (
              <Link
                className="text-sm text-primary underline"
                to={`/stacks/${encodeURIComponent(inventoryId(instance.environmentId, r.projectId))}?tab=editor`}
              >
                {ui("stackEditor")}
              </Link>
            )}
            {r.containerName && <p className="text-xs">{r.containerName}</p>}
          </div>
          <div className="min-w-0 text-sm [overflow-wrap:anywhere]">
            <p>{r.service}</p>
            {r.upstreams?.map(u => (
              <p key={u} className="break-all font-mono text-xs">
                {u}
              </p>
            ))}
            {r.serviceFile && !r.upstreams?.length && (
              <p className="text-xs">{ui("opaqueService", { name: r.serviceFile })}</p>
            )}
          </div>
          <div className="grid content-start gap-2">
            {hashes?.[r.file] && <CertificateCheck instance={instance} route={r} hash={hashes[r.file] ?? ""} />}
            {canEdit(r) && writable && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button variant="ghost" size="icon" aria-label={ui("actions", { name: r.name })}>
                    <MoreHorizontal />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem onSelect={() => edit(r)}>{ui("edit")}</DropdownMenuItem>
                  {instance.kind === "traefik" && (r.dependants?.length ?? 0) > 1 && !!r.upstreams?.length && (
                    <DropdownMenuItem onSelect={() => action({ kind: "edit-service", target: r.file, route: r })}>
                      {ui("editService")}
                    </DropdownMenuItem>
                  )}
                  <DropdownMenuItem onSelect={() => action({ kind: "delete-route", target: r.file, route: r })}>
                    {ui("delete")}
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            )}
          </div>
        </article>
      ))}
    </div>
  );
}
