import type { Tone, TopologyNetwork } from "@/domain/model";

/** LAN networks lime, internal violet, host/none orange, Docker's own bridge grey, other bridges cyan. */
export function networkTone(n: Pick<TopologyNetwork, "driver" | "internal" | "builtIn">): Tone {
  if (n.driver === "macvlan" || n.driver === "ipvlan") return "lime";
  if (n.internal) return "violet";
  if (n.driver === "host" || n.driver === "none") return "orange";
  return n.builtIn ? "gray" : "cyan";
}
