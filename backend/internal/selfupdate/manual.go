// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Exact operator recovery commands from recorded Compose inputs.

package selfupdate

import "strings"

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

// ManualCommands are data for display; the application never executes them.
// Restoration remains an explicit operator action, including after failed health.
func ManualCommands(replay Replay, op *Operation) []string {
	prefix := "docker compose -p " + shellQuote(replay.Project)
	for _, file := range replay.ConfigFiles {
		prefix += " -f " + shellQuote(file)
	}
	prefix += " --env-file " + shellQuote(replay.EnvFile)
	commands := []string{}
	if op != nil && op.Phase == "settled" && op.Outcome != "healthy" {
		commands = append(commands, "cp -- "+shellQuote(replay.EnvFile+".docklet-previous."+op.OpID)+" "+shellQuote(replay.EnvFile))
	}
	return append(commands, prefix+" pull "+shellQuote(replay.Service), prefix+" up -d --no-deps "+shellQuote(replay.Service))
}
