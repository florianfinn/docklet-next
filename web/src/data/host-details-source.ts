// Source: docklet-next/issues/579
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original source fence for host detail reads (M37 #579).

let epoch = 0;

/** Called for every hosts or settings change notification: host details read before it are superseded. */
export function noteHostDetailsChange(): void { epoch++; }

export function hostDetailsEpoch(): number { return epoch; }
