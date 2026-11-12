// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original owning-host container and Compose stop-intent producers.

package project

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/runtimeintent"
	"github.com/moby/moby/client"
	"strings"
	"time"
)

func (s *ProjectService) RecordContainerStopIntent(ctx context.Context, id, name, composeProject string) error {
	// Docker has already stopped the container; finish its intent despite request cancellation.
	durable, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	var stored Project
	projectID := ""
	if composeProject != "" {
		if err := s.db.WithContext(durable).Where("compose_project_name=? OR ((compose_project_name IS NULL OR compose_project_name='') AND name=?)", composeProject, composeProject).Find(&stored).Error; err != nil {
			return err
		}
		projectID = stored.ID
	}
	return runtimeintent.Record(ctx, s.db, []runtimeintent.Row{{ContainerID: id, ContainerName: strings.TrimPrefix(name, "/"), ProjectID: projectID}}, time.Now().UTC())
}

func (s *ProjectService) runningStopIntents(ctx context.Context, projectID, composeName string) ([]runtimeintent.Row, error) {
	cli, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	list, err := cli.ContainerList(ctx, client.ContainerListOptions{Filters: make(client.Filters).Add("label", "com.docker.compose.project="+composeName)})
	if err != nil {
		return nil, err
	}
	rows := []runtimeintent.Row{}
	for _, c := range list.Items {
		// Restart loops and paused containers are stopped by the operation too, so they stay grey (K1).
		if c.State != "running" && c.State != "restarting" && c.State != "paused" {
			continue
		}
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		rows = append(rows, runtimeintent.Row{ContainerID: c.ID, ContainerName: name, ProjectID: projectID})
	}
	return rows, nil
}
