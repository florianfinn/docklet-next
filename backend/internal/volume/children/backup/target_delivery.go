// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original all-host reference fencing and metadata delivery callbacks.

package backup

import (
	"context"
	"net/http"
	"net/url"
	"slices"

	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
)

func (s *Service) configureTargetDelivery(environments *environment.EnvironmentService) {
	s.targets.ConfigureCheckRefresh(func(ctx context.Context) error { return s.refreshTargetChecks(ctx, environments.ProxyJSONRequest) })
	s.targets.ConfigureDelivery(func(ctx context.Context, target backuptarget.Target) (dto.TargetReferences, []string, error) {
		return s.referencesAcrossHosts(ctx, target, environments)
	}, func(ctx context.Context) error {
		var hosts []struct {
			ID      string
			Enabled bool
		}
		if err := s.deps.DB.WithContext(ctx).Table("environments").Where("id <> ?", "0").Select("id,enabled").Find(&hosts).Error; err != nil {
			return err
		}
		for _, host := range hosts {
			if !host.Enabled {
				continue
			}
			_, err := backuptarget.SyncHost(ctx, s.deps.DB, host.ID, environments.ProxyJSONRequest)
			status := ""
			if err != nil {
				status = "backup.target.sync-failed"
			}
			if err = s.deps.DB.WithContext(ctx).Exec("INSERT INTO settings(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value", "backupTargetDeliveryError:"+host.ID, status).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
func (s *Service) referencesAcrossHosts(ctx context.Context, target backuptarget.Target, environments *environment.EnvironmentService) (dto.TargetReferences, []string, error) {
	result := dto.TargetReferences{Policies: []string{}, Backups: []string{}, Running: []string{}}
	unknown := []string{}
	hosts := []string{"0"}
	if target.HostID != nil {
		hosts = []string{*target.HostID}
	} else {
		var remote []string
		if err := s.deps.DB.WithContext(ctx).Table("environments").Where("id <> ?", "0").Order("id").Pluck("id", &remote).Error; err != nil {
			return result, nil, err
		}
		hosts = append(hosts, remote...)
	}
	for _, host := range hosts {
		var refs dto.TargetReferences
		if host == "0" {
			local, err := s.targetReferences(ctx, target.ID)
			if err != nil {
				return result, nil, err
			}
			refs = local
		} else {
			var response base.ApiResponse[dto.TargetReferences]
			if environments == nil {
				unknown = append(unknown, host)
				continue
			}
			if err := environments.ProxyJSONRequest(ctx, host, http.MethodGet, "/api/environments/0/volumes/backups/targets/"+url.PathEscape(target.ID)+"/references", nil, &response); err != nil || !response.Success {
				unknown = append(unknown, host)
				continue
			}
			refs = response.Data
		}
		result.Policies = append(result.Policies, refs.Policies...)
		result.Backups = append(result.Backups, refs.Backups...)
		result.Running = append(result.Running, refs.Running...)
	}
	slices.Sort(unknown)
	return result, unknown, nil
}
