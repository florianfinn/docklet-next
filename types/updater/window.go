// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original daily and weekly policy window wire contract.

package updater

import (
	"fmt"
	"regexp"
)

type UpdateWindow struct {
	Kind string `json:"kind" enum:"daily,weekly"`
	Day  *int   `json:"day,omitempty" minimum:"0" maximum:"6"`
	Time string `json:"time" pattern:"^([01][0-9]|2[0-3]):[0-5][0-9]$"`
}

var windowTime = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

func (w *UpdateWindow) Valid() bool {
	return w != nil && windowTime.MatchString(w.Time) && ((w.Kind == "daily" && w.Day == nil) || (w.Kind == "weekly" && w.Day != nil && *w.Day >= 0 && *w.Day <= 6))
}

func (w *UpdateWindow) Cron() string {
	if !w.Valid() {
		return ""
	}
	day := "*"
	if w.Day != nil {
		day = fmt.Sprint(*w.Day)
	}
	var hour, minute int
	_, _ = fmt.Sscanf(w.Time, "%d:%d", &hour, &minute)
	return fmt.Sprintf("0 %d %d * * %s", minute, hour, day)
}
