// Source: docklet-next/issues/438
// Revision: cc93f43359147e8e128b2ac00bcd410c055eb423
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original candidate proof consumer and verified update integration.

import { createContext, useContext } from "react";
import type { CandidateResults } from "@/data/vulnerability-candidates";

export const CandidateContext = createContext<{ data?: CandidateResults; unavailable?: boolean; reload: () => void } | undefined>(undefined);
export const useCandidateContext = () => useContext(CandidateContext);
