import { Lock } from "lucide-react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { formatPorts, formatRelative } from "@/domain/format";
import type { ContainerInfo, Host } from "@/domain/model";
import { versionLabel } from "@/domain/updates";
import { isFileBind, mountRef } from "@/domain/volumes";
import { filesHref } from "@/features/volumes/files/files-href";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { Alert, AlertDescription, AlertTitle } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";

function Row({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="min-w-0 break-words">{children}</dd>
    </>
  );
}

/** Image, ports, networks, restart policy, mounts (linked to their file workspace) and environment; shared by inspector and container page. */
export function ContainerFacts({ c, host, env = true }: { c: ContainerInfo; host: Host | undefined; env?: boolean }) {
  const t = useTranslations("containers.inspector");
  const tk = useTranslations("volumes.kind");
  return (
    <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm" data-slot="container-facts">
      <Row label={t("image")}>
        <span className="font-mono text-xs">{c.image.repository}</span>
      </Row>
      <Row label={t("version")}>
        <span className="font-mono text-xs">{versionLabel(c.image)}</span>
      </Row>
      <Row label={t("ports")}>
        <span className="font-mono text-xs">{formatPorts(c.ports) || t("none")}</span>
      </Row>
      <Row label={t("networks")}>{c.networks.join(", ") || t("none")}</Row>
      <Row label={t("restart")}>
        <span className="font-mono text-xs">{c.restartPolicy}</span>
      </Row>
      <Row label={t("started")}>{c.startedAt && c.state === "running" ? formatRelative(c.startedAt) : t("none")}</Row>
      <Row label={t("mounts")}>
        {c.mounts.length ? (
          <ul className="flex flex-col gap-1 font-mono text-xs">
            {c.mounts.map((mnt) => {
              const ref = mountRef(c, mnt, host?.projectDir);
              return (
                <li key={mnt} className="flex items-start gap-1.5 break-all">
                  {ref ? (
                    <Link to={filesHref({ id: ref.id, source: ref.name, fileBind: ref.kind === "bind" && isFileBind(ref.name) })} className="hover:text-primary-text hover:underline" aria-label={t("mountOpen", { mount: mnt })}>
                      {mnt}
                    </Link>
                  ) : (
                    mnt
                  )}
                  {ref && <span className="shrink-0 rounded border px-1 font-sans text-[10px] text-muted-foreground">{tk(ref.kind)}</span>}
                </li>
              );
            })}
          </ul>
        ) : (
          t("none")
        )}
      </Row>
      {env && (
        <Row label={t("env")}>
          {Object.keys(c.env).length ? (
            <ul className="flex flex-col gap-0.5 font-mono text-xs">
              {Object.entries(c.env).map(([k, v]) => (
                <li key={k} className="break-all">
                  {k}={v}
                </li>
              ))}
            </ul>
          ) : (
            t("none")
          )}
        </Row>
      )}
    </dl>
  );
}

/** Origin notes: Unraid keeps editing locked; unclear containers can be assigned. */
export function ContainerProvenance({ c }: { c: ContainerInfo }) {
  const t = useTranslations("containers.inspector");
  const to = useTranslations("containers.origin");
  const cmd = useContainerCommands();
  if (c.origin === "unraid")
    return (
      <Alert className="border-orange/30 bg-orange/8">
        <Lock className="text-orange" />
        <AlertTitle className="text-orange">{to("unraidTitle")}</AlertTitle>
        <AlertDescription>{to("unraidBody")}</AlertDescription>
      </Alert>
    );
  if (c.origin === "unknown")
    return (
      <Alert className="border-warning/30 bg-warning/8">
        <Lock className="text-warning" />
        <AlertTitle className="text-warning">{t("assign")}</AlertTitle>
        <AlertDescription className="flex flex-col items-start gap-2">
          {t("assignHint")}
          <div className="flex gap-2">
            <Button size="sm" variant="outline" onClick={() => void cmd.setOrigin(c.id, "unraid")}>
              {t("assignUnraid")}
            </Button>
            <Button size="sm" variant="outline" onClick={() => void cmd.setOrigin(c.id, "standalone")}>
              {t("assignStandalone")}
            </Button>
          </div>
        </AlertDescription>
      </Alert>
    );
  return null;
}
