import { MembershipEdge } from "./TopologyEdges";
import { ContainerNode, LaneNode, NetworkNode } from "./TopologyNodes";

export const NODE_TYPES = { network: NetworkNode, container: ContainerNode, lane: LaneNode };

export const EDGE_TYPES = { membership: MembershipEdge };
