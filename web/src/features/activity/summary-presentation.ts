// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original list-only discovery, bounded reads and demand regression coverage (#225, #223).

/** Old agents omit the list projection; an empty compatibility array alone
 * cannot establish that their step history is known empty. */
export type SummaryView<T> = T & { summary?: { stepsAvailable: boolean } };
export const summaryStepsAvailable = (run: object) => (run as SummaryView<object>).summary?.stepsAvailable !== false;
