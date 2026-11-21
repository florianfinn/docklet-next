// Source: docklet-next/issues/493
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original bounded probe client and certificate observation integration.

import { useEffect, type ReactNode } from "react";
import { CancelledError, useQuery, useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { queryKeys } from "@/data/query-keys";
import { usePrincipal } from "@/data/use-principal";
import { identityLifetime } from "@/data/identity-lifetime";
import { useIdentityVersion, useModuleEnabled } from "@/data/use-module";
import { Button } from "@/shared/ui/button";
import { ProxyError } from "@/data/http/proxy-api";
import { ErrorView } from "./controls";
import { CertificateObservation } from "./CertificateObservation";
import { checkCertificate } from "./certificate-api";
import type { Instance, Route } from "./types";

function certificateHost(instance: Instance, route: Route): string | undefined {
  const host = instance.kind === "caddy" ? route.rule : /Host\(`([^`]+)`\)/.exec(route.rule)?.[1];
  return host && !host.startsWith("*.") ? host : undefined;
}
export function CertificateCheck({ instance, route, hash, frame }: { instance: Instance; route: Route; hash: string; frame?: (content: ReactNode) => ReactNode }) {
  const t = useTranslations("proxyContract.ui"), adapter = useAdapter(), principal = usePrincipal();
  const version = useIdentityVersion(), enabled = useModuleEnabled("proxy"), cache = useQueryClient();
  const host = certificateHost(instance, route);
  const principalId = principal.data?.id;
  const allowed = enabled && !!principalId && principal.data?.isAdmin === true;
  const root = ["module", "proxy", version, "certificate", principalId, instance.environmentId, instance.id, route.file, route.name];
  const query = useQuery<{ observation: Awaited<ReturnType<typeof checkCertificate>>; hash: string }>({
    queryKey: [...root, instance.identity.containerId, instance.identity.startedAt, hash],
    enabled: false, retry: false, placeholderData: (previous, previousQuery) => previousQuery?.queryKey[2] === version && previousQuery?.queryKey[4] === principalId ? previous : undefined,
    queryFn: async ({ signal }) => {
      if (!allowed || !host) throw new CancelledError();
      const observation = await checkCertificate(instance.id, { file: route.file, router: route.name, sourceHash: hash, host }, signal);
      signal.throwIfAborted();
      const states = cache.getQueryData<{ proxy?: boolean }>(["module-states", version]);
      const owner = cache.getQueryData<{ id?: string; isAdmin?: boolean }>(queryKeys.currentUser);
      if (identityLifetime(adapter).version() !== version || !identityLifetime(adapter).active() || states?.proxy !== true || owner?.id !== principalId || owner?.isAdmin !== true) throw new CancelledError();
      return { observation, hash };
    },
  });
  useEffect(() => {
    if (!allowed) {
      void cache.cancelQueries({ queryKey: ["module", "proxy", version, "certificate"] });
      cache.removeQueries({ queryKey: ["module", "proxy", version, "certificate"] });
    }
  }, [allowed, cache, version]);
  useEffect(() => {
    const key = ["module", "proxy", version, "certificate", principalId, instance.environmentId, instance.id, route.file, route.name];
    return () => {
      void cache.cancelQueries({ queryKey: key });
      cache.removeQueries({ queryKey: key });
    };
  }, [cache, version, principalId, instance.environmentId, instance.id, route.file, route.name]);
  if (!allowed || !host || !hash) return null;
  const value = query.data?.observation;
  const outdated = !!value && (value.epoch.containerId !== instance.identity.containerId || value.epoch.startedAt !== instance.identity.startedAt || query.data?.hash !== hash);
  const content = (
    <div className="grid min-w-0 gap-2">
      <Button className="justify-self-end" variant="outline" disabled={query.isFetching} onClick={() => void query.refetch()}>{t("checkCertificate")}</Button>
      {query.error && (query.error instanceof ProxyError && query.error.code === "certificate-unavailable" ? <p>{t("certificateUnavailable")}</p> : <ErrorView error={query.error} />)}
      {value && <CertificateObservation value={value} outdated={outdated} />}
    </div>
  );
  return frame ? frame(content) : content;
}
