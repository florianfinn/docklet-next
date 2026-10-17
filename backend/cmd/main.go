// Source: getarcaneapp/arcane/backend/cmd/main.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; embed schedule time zones in hub and agent.

package main

import (
	_ "time/tzdata"

	"github.com/florianfinn/docklet-next/backend/cli"
)

//	@title		Arcane API
//	@version	1.0
//	@description.markdown

func main() {
	cli.Execute()
}
