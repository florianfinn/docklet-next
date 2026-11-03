// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original hub-only history sampling and retention schedules.

package scheduler

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/history"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"time"
)

type HistoryJob struct {
	Service   *history.Service
	Retention bool
}

func (j *HistoryJob) Name() string {
	if j.Retention {
		return "history-retention"
	}
	return "history-sample"
}
func (j *HistoryJob) Schedule(context.Context) string {
	if j.Retention {
		return "0 0 * * * *"
	}
	return "0 * * * * *"
}
func (j *HistoryJob) Run(ctx context.Context) (st.Outcome, error) {
	var err error
	if j.Retention {
		err = j.Service.Retain(ctx, time.Now().UTC())
	} else {
		err = j.Service.SampleAll(ctx, time.Now().UTC())
	}
	return st.Outcome{Status: st.Succeeded}, err
}
func (*HistoryJob) Reconcile(context.Context, st.Run) (st.Outcome, error) {
	return st.Outcome{Status: st.Skipped}, nil
}
