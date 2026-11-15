// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original optional-module lazy route boundary.

import type { ComponentType, LazyExoticComponent } from "react";
import type { ModuleID } from "@/domain/modules";
import { useModuleEnabled } from "@/data/use-module";
import { NotFound } from "./shell/NotFound";

/** Route registration creates React.lazy once; its importer runs only when
 * the enabled branch renders. Disabled routes keep the standard NotFound. */
export function ModuleRoute({ id, page: Page }: { id: ModuleID; page: LazyExoticComponent<ComponentType> }) {
  const enabled = useModuleEnabled(id);
  return enabled ? <Page /> : <NotFound />;
}
