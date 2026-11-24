// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original contract-gated read-only binding explanations.

import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { useHosts, useStack } from "@/data/queries";
import { useCapability } from "@/data/use-capability";
import { splitInventoryId } from "@/data/http/inventory-mapping";
import { hasUpdate } from "@/domain/updates";
import type { ImageBinding } from "@/data/http/update-binding";
import { BindingRefusal } from "./binding-refusal";

/** Bindings that refuse a selection: project scope, the selected service, or for
 * the whole stack only services with a pending update (the server skips the rest).
 * An unknown update set keeps every service binding. */
export function blockingBindings(bindings: ImageBinding[], selection: string, updatable: ReadonlySet<string> | undefined) {
  return bindings.filter((binding) => binding.scope === "project" || (selection === "*" ? !updatable || updatable.has(binding.service ?? "") : binding.service === selection));
}

export function useStackEligibility(stackId: string, enabled = true) {
  const adapter = useAdapter(), hosts = useHosts(), hostId = stackId.includes("~") ? splitInventoryId(stackId)[0] : "";
  const read = useCapability("getUpdateBaseline", { kind: "stack", id: stackId });
  const host = hosts.data?.find((row) => row.id === hostId);
  const supported = hostId === "0" || (host?.componentVersion?.agentContractVersion ?? 0) >= 23;
  const query = useQuery({ queryKey: ["stack", stackId, "update-eligibility"], queryFn: () => adapter.getUpdateEligibility!(stackId), enabled: adapter.kind === "http" && enabled && supported && read.available && !!adapter.getUpdateEligibility, staleTime: 0 });
  const stack = useStack(stackId), services = stack.data?.services;
  const updatable = services ? new Set(services.filter((service) => hasUpdate(service.update)).map((service) => service.name)) : undefined;
  // Only a current successful read predicts; data retained after a failed refetch or
  // a version change to an agent without the read predicts nothing.
  const data = enabled && supported && read.available && query.status === "success" ? query.data : undefined;
  const bindings = data?.project ? [data.project] : (data?.services ?? []).flatMap((row) => row.binding ? [row.binding] : []);
  // The whole stack stays offered while one service with an update can still be installed alone.
  const blocksAll = bindings.some((binding) => binding.scope === "project") || (!!updatable && updatable.size > 0 && [...updatable].every((name) => bindings.some((binding) => binding.service === name)));
  return { ...query, bindings, updatable, blocksAll, checking: enabled && !!adapter.getUpdateEligibility && read.available && ((hostId !== "0" && hosts.isPending) || supported && query.isPending) };
}
export function StackBindingNotice({ stackId, enabled = true }: { stackId: string; enabled?: boolean }) {
  const eligibility = useStackEligibility(stackId, enabled);
  // Old agents, unknown versions and failed reads predict nothing; a real refusal explains itself.
  if (!enabled) return null;
  // Every current binding is explained, also for a service the update leaves out (#594).
  return <>{eligibility.bindings.map((binding, index) => <BindingRefusal key={`${binding.scope}:${binding.service ?? index}:${binding.reason}`} binding={binding} />)}</>;
}
