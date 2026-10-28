// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original repository mount plans; AUTO-9 helper witness uses identical private binds.

package backuptarget

import (
	"fmt"
	"path"
	"strings"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
)

const VolumeLabel = "docklet.backup-target"

type MountPlan struct {
	Mounts         []mount.Mount
	Volume         *client.VolumeCreateOptions
	RepositoryPath string
	Witness        bool
}

func Plan(target dto.SyncedTarget, hostID string, readOnly bool) MountPlan {
	plan := MountPlan{RepositoryPath: path.Join("/target", target.Path, "docklet-backups", hostID, "volumes"), Witness: target.Kind == "mount"}
	switch target.Kind {
	case "local", "mount":
		plan.RepositoryPath = path.Join("/target", "docklet-backups", hostID, "volumes")
		plan.Mounts = []mount.Mount{{Type: mount.TypeBind, Source: target.Path, Target: "/target", ReadOnly: readOnly, BindOptions: &mount.BindOptions{Propagation: mount.PropagationRPrivate}}}
		if plan.Witness {
			plan.Mounts = append(plan.Mounts, mount.Mount{Type: mount.TypeBind, Source: path.Dir(target.Path), Target: "/parent", ReadOnly: true, BindOptions: &mount.BindOptions{Propagation: mount.PropagationRPrivate}})
		}
	case "smb", "nfs":
		name := fmt.Sprintf("docklet-target-%s-%d", target.ID, target.ConfigVersion)
		options := map[string]string{"type": target.Kind, "device": ":" + target.Share, "o": "addr=" + target.Server}
		if target.Kind == "smb" {
			options["type"] = "cifs"
			options["device"] = "//" + target.Server + "/" + target.Share
			options["o"] += ",username=" + target.Username + ",password=" + target.Secret
		}
		if target.Options != "" {
			options["o"] += "," + target.Options
		}
		plan.Volume = &client.VolumeCreateOptions{Name: name, Driver: "local", DriverOpts: options, Labels: map[string]string{libarcane.InternalResourceLabel: "true", VolumeLabel: target.ID}}
		plan.Mounts = []mount.Mount{{Type: mount.TypeVolume, Source: name, Target: "/target", ReadOnly: readOnly}}
	}
	return plan
}
func CheckWitness(output string) error {
	fields := strings.Fields(output)
	if len(fields) != 2 || fields[0] == fields[1] {
		return dto.NewProblem(422, "backup.target.not-mounted", nil)
	}
	return nil
}
