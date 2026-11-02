// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original live search-parameter access for late workspace command completions (M16 review CFR3-1).

import { useContext } from "react";
import { UNSAFE_DataRouterContext } from "react-router";

/**
 * Pathname and search parameters of the router's current or pending location. After a navigation the router runs ahead
 * of the last React commit, so late command completions read and update the URL here instead of through a rendered
 * snapshot. The location may already belong to another page; callers compare its pathname with their own.
 */
export function useLiveSearch() {
  const context = useContext(UNSAFE_DataRouterContext);
  if (!context) throw new Error("useLiveSearch requires a data router");
  const { router } = context;
  const location = () => router.state.navigation.location ?? router.state.location;
  const pathname = () => location().pathname;
  const read = () => new URLSearchParams(location().search);
  /** Navigates to `target` with `params`; the live hash is kept when the live location is already `target`. */
  const navigate = (target: string, params: URLSearchParams) => {
    const search = params.toString();
    void router.navigate({ pathname: target, search: search ? `?${search}` : "", hash: pathname() === target ? location().hash : "" });
  };
  return { pathname, read, navigate };
}
