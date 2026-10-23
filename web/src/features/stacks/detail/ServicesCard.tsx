import { useCapability } from "@/data/use-capability";
import { Boxes, FileText, RotateCw } from "lucide-react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { formatPorts } from "@/domain/format";
import type { Host, Service, StackDetail } from "@/domain/model";
import { RUNTIME_TONE, serviceRuntime } from "@/domain/status";
import { versionLabel } from "@/domain/updates";
import { ContainerLoad } from "@/shared/components/container-load";
import { SimulationBadge } from "@/shared/components/simulation-badge";
import { IconTile, ToneBadge, ToneDot } from "@/shared/components/tone";
import { UpdateIndicator } from "@/shared/components/updates/update-indicator";
import { Button } from "@/shared/ui/button";
import { Card, CardAction, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { requestServiceRestart } from "@/shared/components/service-restart-store";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";

export function ServicesCard({ stack, host }: { stack: StackDetail; host: Host | undefined }) {
  const t = useTranslations("stacks.overview");
  const ts = useTranslations("common.status");
  const navigate = useNavigate();
  const cmd = useStackCommands();
  const restart = useCapability("restartService");
  const rowFor = (svc: Service) => {
    const rt = serviceRuntime(svc, host);
    const tone = RUNTIME_TONE[rt];
    return (
      <TableRow key={svc.name} className="group">
        <TableCell>
          <span className="flex items-center gap-2 font-medium">
            <ToneDot tone={tone} pulse={rt === "unhealthy"} />
            {svc.name}
          </span>
        </TableCell>
        <TableCell>
          <ToneBadge tone={tone}>{ts(rt)}</ToneBadge>
        </TableCell>
        <TableCell className="max-w-56">
          <span className="block truncate font-mono text-xs">{versionLabel(svc.image)}</span>
          <span className="block truncate text-[11px] text-muted-foreground">{svc.image.repository}</span>
        </TableCell>
        <TableCell className="font-mono text-xs text-muted-foreground">{formatPorts(svc.ports) || "–"}</TableCell>
        <TableCell>
          <ContainerLoad id={`c-${stack.id}-${svc.name}`} memLimitMb={svc.memLimitMb} unavailable={host?.status !== "online" || svc.state !== "running"} />
        </TableCell>
        <TableCell>
          <UpdateIndicator
            name={svc.name}
            update={svc.update}
            actions={{ updateOperation: "installStackUpdates", onUpdate: () => void cmd.installUpdates(stack.id, [svc.name]), onRecheck: () => void cmd.checkUpdates(stack.id), updateLabel: undefined, busy: !!stack.busyActionId }}
          />
        </TableCell>
        <TableCell className="text-right">
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="size-8" onClick={() => navigate(`/stacks/${stack.id}?tab=logs&service=${svc.name}`)} aria-label={t("logs", { name: svc.name })}>
                <FileText />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("logs", { name: svc.name })}</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <Button variant="ghost" size="icon" className="group/btn size-8" disabled={!restart.available || !!stack.busyActionId} title={restart.reason} onClick={() => requestServiceRestart([{ stackId: stack.id, stackName: stack.name, service: svc.name }], () => cmd.restartService(stack.id, svc.name))} aria-label={t("restart", { name: svc.name })}>
                <RotateCw className="transition-transform duration-(--dur-slow) group-hover/btn:rotate-90" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("restart", { name: svc.name })}</TooltipContent>
          </Tooltip>
        </TableCell>
      </TableRow>
    );
  };
  return (
    <Card className="gap-2 py-4">
      <CardHeader className="px-4">
        <CardTitle className="flex items-center gap-2.5">
          <IconTile tone="info" size="sm">
            <Boxes />
          </IconTile>
          {t("services")}
        </CardTitle>
        <CardAction>
          <SimulationBadge />
        </CardAction>
      </CardHeader>
      <CardContent className="overflow-x-auto px-2">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("columns.service")}</TableHead>
              <TableHead>{t("columns.status")}</TableHead>
              <TableHead>{t("columns.image")}</TableHead>
              <TableHead>{t("columns.ports")}</TableHead>
              <TableHead>
                {t("columns.cpu")} / {t("columns.mem")}
              </TableHead>
              <TableHead>{t("columns.update")}</TableHead>
              <TableHead className="text-right">
                <span className="sr-only">{t("columns.actions")}</span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>{stack.services.map(rowFor)}</TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}
