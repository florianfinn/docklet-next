// Source: getarcaneapp/arcane/types/env/env.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed central variables and .env.global.

package env

type Variable struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Summary struct {
	Variables []Variable `json:"variables"`
}
