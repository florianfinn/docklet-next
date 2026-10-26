// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Two hub coordinator jobs per registered host; no inherited local watcher.

package scheduler

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/updater"
	st "github.com/florianfinn/docklet-next/types/scheduler"
)

type ImagePolicyJob struct {
	Owner               *updater.UpdaterService
	EnvironmentID, Kind string
}

func (j *ImagePolicyJob) Name() string { return updater.ImagePolicyJobKey(j.EnvironmentID, j.Kind) }
func (j *ImagePolicyJob) Schedule(ctx context.Context) string {
	row, err := j.Owner.ImagePolicySchedule(ctx, j.EnvironmentID)
	if err != nil {
		return "0 0 0 * * *"
	}
	if j.Kind == "image-polling" {
		return row.PollingInterval
	}
	return row.AutoUpdateInterval
}
func (j *ImagePolicyJob) ShouldSchedule(ctx context.Context) bool {
	row, err := j.Owner.ImagePolicySchedule(ctx, j.EnvironmentID)
	return err == nil && j.Owner.ImagePolicyEnabled(ctx, row, j.Kind)
}
func (j *ImagePolicyJob) Run(ctx context.Context) (st.Outcome, error) {
	return j.Owner.RunImagePolicyHost(ctx, j.EnvironmentID, j.Kind)
}
func (j *ImagePolicyJob) Reconcile(ctx context.Context, run st.Run) (st.Outcome, error) {
	return j.Owner.ReconcileImagePolicyHost(ctx, j.EnvironmentID, run)
}
func (*ImagePolicyJob) ValidateRetry(context.Context, st.Run) error {
	return errors.New("A policy run cannot replay prior update effects; use the next ordinary schedule")
}

func RefreshImagePolicyJobs(ctx context.Context, controller st.JobScheduler, owner *updater.UpdaterService) error {
	hosts, err := owner.ImagePolicyHosts(ctx)
	if err != nil {
		return err
	}
	wanted := map[string]bool{}
	for _, host := range hosts {
		for _, kind := range []string{"image-polling", "auto-update"} {
			job := &ImagePolicyJob{Owner: owner, EnvironmentID: host, Kind: kind}
			wanted[job.Name()] = true
			if err = controller.AddJob(ctx, job); err != nil {
				return err
			}
		}
	}
	for _, job := range controller.ListRegisteredJobs() {
		if _, _, owned := updater.ParseImagePolicyJob(job.Name()); owned && !wanted[job.Name()] {
			controller.RemoveJob(ctx, job.Name())
		}
	}
	return nil
}
