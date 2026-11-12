// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original identity-bound bulk summary ownership across navigation. M19 web consumer integration.

import { createStore } from "@/shared/lib/store";
import type { BulkResult } from "@/shared/hooks/bulk-run";
export const bulkResultStore = createStore<{ open: boolean; results: BulkResult[] }>({ open: false, results: [] });
