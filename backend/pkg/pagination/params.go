// Source: getarcaneapp/arcane/backend/pkg/pagination/params.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package pagination

type QueryParams struct {
	SearchQuery
	SortParams
	Params

	Filters map[string]string
}
