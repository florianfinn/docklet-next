// Source: docklet-next/issues/338
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original independent sustained-load metric episodes.

package history

type episode struct {
	high, low int
	alerted   bool
	last      int64
}

func evaluateLoad(e episode, minute int64, value *float64, threshold float64, minutes int) (episode, bool) {
	if e.last != minute-60 {
		e.high, e.low = 0, 0
	}
	e.last = minute
	if value == nil {
		e.high, e.low = 0, 0
		return e, false
	}
	if *value > threshold {
		e.high++
		e.low = 0
		if e.high >= minutes && !e.alerted {
			e.alerted = true
			return e, true
		}
	} else {
		e.high = 0
		e.low++
		if e.low >= minutes {
			e.alerted = false
		}
	}
	return e, false
}
