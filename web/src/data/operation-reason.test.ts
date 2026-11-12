// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

import { expect, it } from "vitest";
import { operationReasonKey } from "./operation-reason";
import { enContainers } from "@/features/containers/messages/en";
import { deContainers } from "@/features/containers/messages/de";
it.each(["not-found", "admin-required", "full-required", "disabled", "not-registered", "unavailable"])("maps access.%s to complete EN/DE messages", (suffix) => {
 const key = operationReasonKey(`access.${suffix}`);
 expect(key).toBeDefined();
 for (const messages of [enContainers.operationReasons, deContainers.operationReasons]) expect(messages[key as keyof typeof messages]).toBeTruthy();
});
