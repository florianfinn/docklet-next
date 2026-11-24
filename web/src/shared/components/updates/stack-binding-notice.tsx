// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original contract-gated read-only binding explanations.

import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { useHosts } from "@/data/queries";
import { useCapability } from "@/data/use-capability";
import { splitInventoryId } from "@/data/http/inventory-mapping";
import { BindingRefusal } from "./binding-refusal";

export function useStackEligibility(stackId: string, enabled = true) {
  const adapter = useAdapter(), hosts = useHosts(), hostId = stackId.includes("~") ? splitInventoryId(stackId)[0] : "";
  const read = useCapability("getUpdateBaseline", { kind: "stack", id: stackId });
  const host = hosts.data?.find((row) => row.id === hostId);
  const supported = hostId === "0" || (host?.componentVersion?.agentContractVersion ?? 0) >= 23;
  const query = useQuery({ queryKey: ["stack", stackId, "update-eligibility"], queryFn: () => adapter.getUpdateEligibility!(stackId), enabled: adapter.kind === "http" && enabled && supported && read.available && !!adapter.getUpdateEligibility, staleTime: 0 });
  const bindings = query.data?.project ? [query.data.project] : (query.data?.services ?? []).flatMap((row) => row.binding ? [row.binding] : []);
  return { ...query, bindings, checking: enabled && !!adapter.getUpdateEligibility && read.available && ((hostId !== "0" && hosts.isPending) || supported && query.isPending), generic: !supported || query.isError };
}
export function StackBindingNotice({ stackId, enabled = true }: { stackId: string; enabled?: boolean }) {
  const eligibility = useStackEligibility(stackId, enabled);
  if (!enabled) return null;
  return eligibility.generic ? <BindingRefusal /> : <>{eligibility.bindings.map((binding, index) => <BindingRefusal key={`${binding.scope}:${binding.service ?? index}:${binding.reason}`} binding={binding} />)}</>;
}
