// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original shared live zone provider contract.

package schedule

import "time"

// ZoneProvider resolves the owning process's current schedule zone per use.
type ZoneProvider interface {
	Location() *time.Location
}

type ZoneFunc func() *time.Location

func (f ZoneFunc) Location() *time.Location { return f() }

func FixedZone(loc *time.Location) ZoneProvider {
	if loc == nil {
		loc = time.UTC
	}
	return ZoneFunc(func() *time.Location { return loc })
}
