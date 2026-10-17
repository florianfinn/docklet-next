// Source: docklet-next/issues/85
// Revision: 37ddf907edabe127fe061523c7aba1c764d87d85
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original acceptance correction and regression coverage.

package container

import containertypes "github.com/florianfinn/docklet-next/types/container"

// The inventory presents a published port once across address families.
func mergePortBindings(ports []containertypes.Port) []containertypes.Port {
	type key struct {
		host, container int
		protocol        string
	}
	seen := make(map[key]int)
	merged := make([]containertypes.Port, 0, len(ports))
	for _, port := range ports {
		k := key{port.PublicPort, port.PrivatePort, port.Type}
		if index, ok := seen[k]; ok {
			if merged[index].IP != port.IP {
				merged[index].IP = ""
			}
			continue
		}
		seen[k] = len(merged)
		merged = append(merged, port)
	}
	return merged
}
