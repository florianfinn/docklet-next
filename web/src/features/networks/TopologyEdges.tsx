import { BaseEdge, type Edge, type EdgeProps } from "@xyflow/react";
import { memo } from "react";

import { membershipPoints, roundedPath } from "./topology-geometry";

export interface MembershipEdgeData extends Record<string, unknown> {
  from: number;
  to: number;
}

/** Network membership routed through free bands and the rail (see membershipPoints). */
export const MembershipEdge = memo(function MembershipEdge({ sourceX, sourceY, targetX, targetY, data, style }: EdgeProps<Edge<MembershipEdgeData, "membership">>) {
  const route = { sx: sourceX, sy: sourceY, tx: targetX, ty: targetY, from: data?.from ?? sourceY, to: data?.to ?? sourceY };
  return <BaseEdge path={roundedPath(membershipPoints(route))} style={style} />;
});
