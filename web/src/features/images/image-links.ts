import { imageIdFor } from "@/domain/images";
import type { ContainerInfo } from "@/domain/model";

/** Images page with the details of the container's image open. */
export function imageHref(c: Pick<ContainerInfo, "hostId" | "image" | "imageId">): string {
  return `/images?image=${encodeURIComponent(c.imageId ?? imageIdFor(c.hostId, c.image.repository, c.image.tag))}`;
}
