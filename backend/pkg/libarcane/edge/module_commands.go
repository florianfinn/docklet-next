// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Require every command behind an advertised optional-module capability.

package edge

import (
	"github.com/florianfinn/docklet-next/types/features"
	"slices"
)

// ModuleCommands derives the work surface from the actual command registry.
// Cleanup is shared management and remains part of compatibility while off.
func ModuleCommands(id features.ID) []string {
	commands := []string{"module.data.delete"}
	for _, route := range commandRoutes {
		if module, ok := features.ForPath(route.PathPattern); ok && module.ID == id && !route.LocalOnly && !slices.Contains(commands, route.CommandName) {
			commands = append(commands, route.CommandName)
		}
	}
	return commands
}
func HasModuleCommands(capabilities []string, id features.ID) bool {
	definition, ok := features.Lookup(id)
	if !ok || definition.Kind != features.Module || !slices.Contains(capabilities, definition.Capability) {
		return false
	}
	for _, command := range ModuleCommands(id) {
		if !slices.Contains(capabilities, command) {
			return false
		}
	}
	return true
}
