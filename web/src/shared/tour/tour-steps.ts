// Source: docklet-next/issues/640
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original guided tour steps and anchors.

import type { NavKey } from "@/app/shell/nav-items";

import type { enTour } from "./messages/en";

export type TourStepKey = keyof (typeof enTour)["steps"];

export interface TourStep {
  key: TourStepKey;
  /** Page of the step; elsewhere the tour points at its navigation entry. Without a route any page works. */
  route?: string;
  /** Navigation label of the route. */
  nav?: NavKey;
  /** Anchor candidates in order; the first visible one is highlighted, none centres the card. */
  anchors: readonly string[];
}

const PAGE_HEADER = "#main-content header";

export const TOUR_STEPS: readonly [TourStep, ...TourStep[]] = [
  { key: "dashboard", nav: "dashboard", route: "/dashboard", anchors: ['[data-slot="fleet-ring"]', PAGE_HEADER] },
  { key: "hostFilter", nav: "dashboard", route: "/dashboard", anchors: ["[data-host-filter]"] },
  { key: "hosts", nav: "hosts", route: "/hosts", anchors: [PAGE_HEADER] },
  { key: "stacks", nav: "stacks", route: "/stacks", anchors: [PAGE_HEADER] },
  { key: "containers", nav: "containers", route: "/containers", anchors: [PAGE_HEADER] },
  { key: "security", nav: "security", route: "/security", anchors: [PAGE_HEADER] },
  { key: "backups", nav: "backups", route: "/backups", anchors: [PAGE_HEADER] },
  { key: "notifications", nav: "notifications", route: "/settings/notifications", anchors: ['section[aria-labelledby="notifications-title"]', PAGE_HEADER] },
  { key: "search", anchors: ['[data-tour="search"]'] },
];
