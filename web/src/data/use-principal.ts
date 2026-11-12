// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original shared current-principal observation for reduced user surfaces.

import { useQuery } from "@tanstack/react-query";
import { queryKeys } from "./query-keys";
import { useAdapter } from "./use-adapter";

/** AuthGate and the identity bridge own reads; consumers only observe their shared cache. */
export function usePrincipal() {
  const adapter = useAdapter();
  return useQuery({ queryKey: queryKeys.currentUser, queryFn: () => adapter.getCurrentUser(), enabled: false });
}

export function useIsUser(): boolean { return usePrincipal().data?.isAdmin === false; }
