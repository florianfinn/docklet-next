// Docklet's own components (the agent) are workloads on the host but not user
// stacks or containers. They are recognised only by an explicit label that the
// installer writes, never by name, so a user stack called "docklet-agent" stays visible.

import type { ContainerState, ImageVersion, LogLevel, PortMapping } from "./model";

export const SYSTEM_LABEL = "io.docklet.system";
export type SystemRole = "agent";

/** Labels the installer puts on the agent's compose project and container. */
export const AGENT_LABELS: Readonly<Record<string, string>> = { [SYSTEM_LABEL]: "agent" };

export function systemRole(labels: Readonly<Record<string, string>> | undefined): SystemRole | undefined {
  return labels?.[SYSTEM_LABEL] === "agent" ? "agent" : undefined;
}

export function isSystemWorkload(item: { dockerLabels?: Readonly<Record<string, string>> }): boolean {
  return systemRole(item.dockerLabels) !== undefined;
}

/** The rule every list, count and discovery result goes through at the adapter boundary. */
export function withoutSystem<T extends { dockerLabels?: Readonly<Record<string, string>> }>(items: readonly T[]): T[] {
  return items.filter((item) => !isSystemWorkload(item));
}

/** Agent container as reported for the host details; never part of stack or container lists. */
export interface SystemComponent {
  /** Production metadata is reported without inventing Docker inspect fields. */
  metadataOnly?: boolean;
  agentVersion?: string;
  contractVersion?: number;
  toolsImage?: string;
  updateRequired?: boolean;
  errorCode?: string;
  hostId: string;
  role: SystemRole;
  containerName: string;
  composeProject: string;
  image: ImageVersion;
  state: ContainerState;
  installDir: string;
  /** Install directory lies inside the project directory, so stack discovery sees it and skips it by label. */
  insideProjectDir: boolean;
  labels: Record<string, string>;
  startedAt?: string;
  ports: PortMapping[];
  mounts: string[];
  networks: string[];
  restartPolicy: string;
  memLimitMb: number;
  /** Recent lines of the agent's own log; synthetic in the demo. */
  recentLog: { at: string; level: LogLevel; message: string }[];
}
