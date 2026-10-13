import { useStacks } from "@/data/queries";
import type { ContainerInfo } from "@/domain/model";
import { requestServiceRestart, type ServiceTarget } from "@/shared/components/service-restart-store";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";

/** The Compose service a container runs, or undefined for standalone, Unraid and unclear containers. */
export function serviceTarget(c: ContainerInfo, stackName: (id: string) => string | undefined): ServiceTarget | undefined {
  if (c.origin !== "compose" || !c.stackId || !c.serviceName) return undefined;
  return { stackId: c.stackId, stackName: stackName(c.stackId) ?? c.stackId, service: c.serviceName, containerId: c.id };
}

/**
 * Restart of one or more containers: standalone ones restart at once as
 * before; containers of a Compose stack first ask, since only their service
 * restarts (see ServiceRestartDialog).
 */
export function useContainerRestart() {
  const cmd = useContainerCommands();
  const stacks = useStacks();
  const name = (id: string) => stacks.data?.find((s) => s.id === id)?.name;
  return (list: ContainerInfo[]) => {
    const targets = list.map((c) => serviceTarget(c, name)).filter((x): x is ServiceTarget => !!x);
    const run = async () => {
      const results = await Promise.all(list.map((c) => cmd.run(c.id, "restart")));
      return results.some((r) => r === undefined) ? undefined : results;
    };
    if (targets.length) requestServiceRestart(targets, run);
    else void run();
  };
}
