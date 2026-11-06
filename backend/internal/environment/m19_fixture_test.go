// Source: docklet-next/issues/352
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original history-condition deletion fixture for environment tests.

package environment

type testHistoryConditionRow struct{ HostID string }

func (testHistoryConditionRow) TableName() string { return "history_conditions" }
