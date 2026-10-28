// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { useSecurity } from "@/data/queries";
import type { DockerImage } from "@/domain/images";

/** The same host-qualified image identity drives list chips, details and scan admission. */
export function useImageSecurity(image: DockerImage) {
  const security = useSecurity();
  const row = security.data?.images.find((r) => r.id === image.id && r.hostId === image.hostId);
  return { counts: security.data?.enabled === false || row?.unavailable || row && !row.scannedAt ? undefined : row?.counts ?? image.vulnerabilities,
    unavailable: security.isError || !!row?.unavailable || !!security.data?.unavailable?.some((h) => h.hostId === image.hostId),
    notScanned: !!row && !row.unavailable && !row.scannedAt,
    platform: row?.platform };
}
