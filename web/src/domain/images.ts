// Docker images per host. Consumers are derived from the containers that
// reference an image, so stacks, containers and images always agree.

import type { ContainerState, InventoryObservation, Severity, UpdateInfo } from "./model";

export interface ImageConsumer {
  containerId: string;
  name: string;
  hostId: string;
  stackId?: string;
  stackName?: string;
  serviceName?: string;
  state: ContainerState;
  /** Docklet's own agent; listed for honesty, never linked as a user workload. */
  system?: boolean;
}

export interface DockerImage {
  inventory?: InventoryObservation;
  usageState?: "known" | "unknown";
  protectionReasons?: { reason: string; ownerId?: string }[];
  updateObservation?: ImageUpdateObservation;
  metadataUnavailable?: boolean;
  id: string;
  hostId: string;
  repository: string;
  /** null for a dangling image (`<none>:<none>`, superseded by a newer pull of the same tag). */
  tag: string | null;
  /** Further tags on the same image. */
  aliases: string[];
  /** Release version mapped from tag or OCI label; absent when unknown. */
  version?: string;
  /** Content digest; shown only in the details. Synthetic in the demo. */
  digest: string;
  sizeMb: number;
  createdAt: string;
  pulledAt?: string;
  architecture: string;
  os: string;
  layers: number;
  consumers: ImageConsumer[];
  /** Update state of the consumers using this image, when they have one. */
  update?: UpdateInfo;
  /** Findings of the last scan for this reference on this host. */
  vulnerabilities?: Record<Severity, number>;
}

export type ImageUsage = "in-use" | "unused" | "dangling" | "unknown";

export function imageUsage(image: Pick<DockerImage, "tag" | "consumers" | "usageState" | "protectionReasons">): ImageUsage {
  if (image.consumers.length || image.protectionReasons?.some((item) => item.reason === "in-use")) return "in-use";
  if (image.usageState === "unknown") return "unknown";
  return image.tag === null ? "dangling" : "unused";
}

export function imageReference(image: Pick<DockerImage, "repository" | "tag">): string {
  return image.tag === null ? `${image.repository}@<none>` : `${image.repository}:${image.tag}`;
}

/** Stable id of a tagged image on a host. */
export function imageIdFor(hostId: string, repository: string, tag: string): string {
  return `img-${hostId}-${`${repository}-${tag}`.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "")}`;
}

const REF = /^(?:[a-z0-9]+(?:[._-][a-z0-9]+)*(?::\d{2,5})?\/)?[a-z0-9]+(?:[._-][a-z0-9]+)*(?:\/[a-z0-9]+(?:[._-][a-z0-9]+)*)*(?::([A-Za-z0-9_][A-Za-z0-9_.-]{0,127}))?$/;

/** Parses "repository[:tag]" as Docker accepts it; digests and uppercase repositories are refused. */
export function parseImageReference(input: string): { repository: string; tag: string } | null {
  const ref = input.trim();
  if (!ref || ref.includes("@") || !REF.test(ref)) return null;
  const at = ref.lastIndexOf(":");
  const slash = ref.lastIndexOf("/");
  if (at > slash) return { repository: ref.slice(0, at), tag: ref.slice(at + 1) };
  return { repository: ref, tag: "latest" };
}

export type PruneMode = "dangling" | "unused";

/** Images a prune would remove: never one with a consumer, running or stopped. */
export function pruneCandidates(images: readonly DockerImage[], mode: PruneMode): DockerImage[] {
  return images.filter((i) => {
    if (imageRemovalBlocked(i)) return false;
    const usage = imageUsage(i);
    return usage === "dangling" || (mode === "unused" && usage === "unused");
  });
}

export function totalSizeMb(images: readonly Pick<DockerImage, "sizeMb">[]): number {
  return images.reduce((n, i) => n + i.sizeMb, 0);
}

/** Short digest for detail views: algorithm and the first 12 hex characters. */
export function shortDigest(digest: string): string {
  const [algo = "", hex = ""] = digest.split(":");
  return hex ? `${algo}:${hex.slice(0, 12)}` : digest;
}

export interface ImagePullProgress {
  phase: "pulling" | "waiting" | "downloading" | "verifying" | "extracting" | "complete";
  current?: number;
  total?: number;
}
export interface ImagePullResult { actionId?: string }
export interface ImagePruneItem { id: string; code: string }
export interface ImagePruneHostReport {
  hostId: string;
  removed: string[];
  skipped: ImagePruneItem[];
  failed: ImagePruneItem[];
  /** Missing when the executor cannot establish reclaimed bytes. */
  spaceReclaimed?: number;
  errorCode?: string;
}
export interface ImagePruneResult { hosts: ImagePruneHostReport[] }
export interface ImageUpdateObservation {
  state: "unchecked" | "current" | "version" | "rebuild" | "local" | "unknown" | "error";
  checkedAt?: string;
  currentVersion?: string;
  candidateVersion?: string;
  code?: string;
}

/** A missing usage scan or any executor protection prevents local destructive controls. */
export function imageRemovalBlocked(image: DockerImage): boolean {
  return image.usageState === "unknown" || !!image.protectionReasons?.length || image.consumers.length > 0 || !!image.inventory?.stale;
}
