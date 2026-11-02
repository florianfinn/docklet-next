// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original live search-parameter access for late workspace command completions (M16 review CFR3-1).

import { useContext } from "react";
import { UNSAFE_DataRouterContext } from "react-router";

/**
 * Search parameters of the router's current or pending location. After a navigation the router runs ahead of the last
 * React commit, so late command completions read and update the URL here instead of through a rendered snapshot.
 */
export function useLiveSearch() {
  const context = useContext(UNSAFE_DataRouterContext);
  if (!context) throw new Error("useLiveSearch requires a data router");
  const { router } = context;
  const location = () => router.state.navigation.location ?? router.state.location;
  const read = () => new URLSearchParams(location().search);
  const update = (change: (params: URLSearchParams) => URLSearchParams) => {
    const { pathname, hash } = location();
    const search = change(read()).toString();
    void router.navigate({ pathname, search: search ? `?${search}` : "", hash });
  };
  return { read, update };
}
