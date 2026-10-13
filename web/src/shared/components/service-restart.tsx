import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useContainers, useTopology } from "@/data/queries";
import { useStore } from "@/shared/lib/store";

import { serviceRestartStore as store, type ServiceRestartRequest as Request, type ServiceTarget } from "./service-restart-store";

import { ConfirmDialog } from "./confirm-dialog";

/** Names of services that wait for or are needed by `containerId` (Compose depends_on, never observed traffic). */
function useDependencies(target: ServiceTarget | undefined, enabled: boolean) {
  const topology = useTopology();
  const containers = useContainers();
  if (!enabled || !target || !topology.data || !containers.data) return { waitsFor: [], neededBy: [] };
  const own = target.containerId ?? containers.data.find((c) => c.stackId === target.stackId && c.serviceName === target.service)?.id;
  const name = (id: string) => {
    const c = containers.data.find((x) => x.id === id);
    return c?.serviceName ?? c?.name ?? id;
  };
  const deps = topology.data.dependencies;
  return {
    waitsFor: deps.filter((d) => d.from === own).map((d) => name(d.to)),
    neededBy: deps.filter((d) => d.to === own).map((d) => name(d.from)),
  };
}

/** The one confirmation dialog for service restarts; mounted once in the shell. */
export function ServiceRestartDialog() {
  const request = useStore(store, (s) => s);
  return request ? <ServiceRestartConfirm request={request} /> : null;
}

function ServiceRestartConfirm({ request }: { request: Request }) {
  const t = useTranslations("common.serviceRestart");
  const single = request.targets.length === 1 ? request.targets[0] : undefined;
  const deps = useDependencies(single, !!single);
  const close = () => store.set(null);
  return (
    <ConfirmDialog
      open
      onOpenChange={(o) => !o && close()}
      title={single ? t("title", { service: single.service }) : t("titleMany", { count: request.targets.length })}
      confirmLabel={single ? t("confirm") : t("confirmMany", { count: request.targets.length })}
      onConfirm={async () => {
        const result = await request.run();
        return result === undefined ? false : result;
      }}
      description={
        <div className="flex flex-col gap-2" data-slot="service-restart">
          {single ? <p>{t("body", { service: single.service, stack: single.stackName })}</p> : <p>{t("bodyMany")}</p>}
          {!single && (
            <ul className="list-disc pl-5">
              {request.targets.map((s) => (
                <li key={`${s.stackId}/${s.service}`}>{t("item", { service: s.service, stack: s.stackName })}</li>
              ))}
            </ul>
          )}
          <p>{t("keeps")}</p>
          {single && deps.neededBy.length > 0 && <p className="text-warning">{t("neededBy", { services: deps.neededBy.join(", ") })}</p>}
          {single && deps.waitsFor.length > 0 && <p>{t("waitsFor", { services: deps.waitsFor.join(", ") })}</p>}
          <p>
            {t("wholeStack")}{" "}
            {single && (
              <Link to={`/stacks/${single.stackId}`} className="text-primary-text underline-offset-4 hover:underline" onClick={close}>
                {t("openStack", { stack: single.stackName })}
              </Link>
            )}
          </p>
        </div>
      }
    />
  );
}

