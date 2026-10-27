// Source: docklet-next/docs/design/images-updates.md
// Revision: 5c1c4dd77cbeb7b5f702e50937bfc64de3c3f9d8
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original stored-observation and deployment-history consumer for issue #231.

import type { components } from "@/api/generated/schema";

export const observation = (state: components["schemas"]["ImageUpdateInfo"]["state"]): components["schemas"]["ImageUpdateInfo"] => ({ state, imageRef: "example:1.0.0", authState: "anonymous", checkedAt: "2026-10-26T00:00:00Z", checkTime: "2026-10-26T00:00:00Z", currentVersion: "1.0.0", latestVersion: "1.1.0", currentDigest: "sha256:old", latestDigest: "sha256:new", error: "private raw diagnostic", hasUpdate: state === "version" || state === "rebuild", responseTimeMs: 1, updateType: "minor" });
