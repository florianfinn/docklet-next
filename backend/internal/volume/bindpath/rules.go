// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package bindpath

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/inventory"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"path"
	"strings"
)

func Overlap(a, b string) bool {
	a, b = path.Clean(a), path.Clean(b)
	return a == b || strings.HasPrefix(a, b+"/") || strings.HasPrefix(b, a+"/") || a == "/" || b == "/"
}

// Self reports Docklet's own hub or agent container: the images and the agent template label it io.docklet.system;
// the Arcane labels cover containers installed from Arcane templates.
func Self(c container.Summary) bool {
	if system := c.Labels[inventory.SystemLabel]; system == "hub" || system == "agent" {
		return true
	}
	return strings.EqualFold(c.Labels["com.getarcaneapp.arcane"], "true") || strings.EqualFold(c.Labels["com.getarcaneapp.arcane.agent"], "true")
}

// Operation containers are infrastructure and cannot admit or protect their own source.
func Helper(c container.Summary) bool {
	return c.Labels[libarcane.InternalResourceLabel] == "true" && !Self(c)
}
func Uses(c container.Summary, source string, overlap bool) bool {
	for _, m := range c.Mounts {
		if m.Type == mount.TypeBind && (m.Source == source || overlap && Overlap(m.Source, source)) {
			return true
		}
	}
	return false
}

func Rules(source, dockerRoot string, rows []container.Summary, targets []string, stored bool) error {
	deny := func(reason string) error {
		e := dto.NewProblem(422, "volume.bind.path-denied", nil)
		e.Reason = reason
		return e
	}
	for _, protected := range []string{"/proc", "/sys", "/dev", "/etc", "/run", "/var/run", "/boot"} {
		if Overlap(source, protected) {
			return deny("system")
		}
	}
	for _, protected := range []string{"/var/lib/docker", "/var/lib/containerd", dockerRoot} {
		if protected != "" && Overlap(source, protected) {
			return deny("docker")
		}
	}
	for _, c := range rows {
		if Self(c) && Uses(c, source, true) {
			return deny("docklet")
		}
	}
	for _, target := range targets {
		if Overlap(source, target) {
			return deny("docklet")
		}
	}
	if !stored {
		for _, c := range rows {
			if !Self(c) && !Helper(c) && Uses(c, source, false) {
				return nil
			}
		}
		return dto.NewProblem(404, "volume.bind.not-mounted", nil)
	}
	return nil
}

// Check reads policy/history for admission only. By-ID reads never call it.
func Check(ctx context.Context, cli *client.Client, db *database.DB, key string) (string, error) {
	source, err := volume.BindSource(key)
	if err != nil {
		return "", err
	}
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return "", err
	}
	rows, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return "", err
	}
	stored := false
	targets := []string{}
	if db != nil {
		for _, table := range []string{"volume_backup_policies", "volume_backups", "volume_restore_runs"} {
			var count int64
			if err = db.WithContext(ctx).Table(table).Where("volume_name = ?", key).Count(&count).Error; err != nil {
				return "", err
			}
			stored = stored || count > 0
		}
		targets, err = TargetPaths(ctx, db)
		if err != nil {
			return "", err
		}
	}
	if err = Rules(source, info.Info.DockerRootDir, rows.Items, targets, stored); err != nil {
		return "", err
	}
	return source, nil
}

func SourceMount(source, kind string, readOnly bool) mount.Mount {
	target := "/volume"
	if kind == "file" {
		target = path.Join(target, path.Base(source))
	}
	return mount.Mount{Type: mount.TypeBind, Source: source, Target: target, ReadOnly: readOnly}
}

// The hub stores all target owners; agents store only definitions delivered for themselves.
func TargetPaths(ctx context.Context, db *database.DB) ([]string, error) {
	result := []string{}
	if db == nil {
		return result, nil
	}
	host := "0"
	var setting struct{ Value string }
	if err := db.WithContext(ctx).Table("settings").Where("key = ?", "hubHostId").Limit(1).Find(&setting).Error; err != nil {
		return nil, err
	}
	if setting.Value != "" {
		host = setting.Value
	}
	err := db.WithContext(ctx).Table("backup_targets").Where("kind IN ? AND (host_id = ? OR host_id = ?)", []string{"local", "mount"}, host, "0").Pluck("path", &result).Error
	return result, err
}
