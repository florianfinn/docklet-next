// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Split from service.go for M26; streamed download seams for #386. Earlier deviations: see service.go.

package backup

import (
	"context"
	"io"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	scheduleutil "github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
)

// Dependencies are the persistence, runtime, and helper-container operations volume backups use.
type Dependencies struct {
	Store          Store
	DB             *database.DB
	Docker         *docker.DockerClientService
	Events         *event.EventService
	Activity       *activity.ActivityService
	Engine         *backup.Engine
	RecoveryKeys   *backup.RecoveryKeyStore
	Locks          *utils.KeyedMutex
	AlreadyRunning error
	StopContainer  func(ctx context.Context, containerID string, user usertypes.Actor) error
	StartContainer func(ctx context.Context, containerID string, user usertypes.Actor) error
	HelperImage    func(ctx context.Context, dockerClient *client.Client) (string, error)
	AcquireHelper  func(ctx context.Context, volumeName string) (string, func(), error)
	Exec           func(ctx context.Context, containerID, execUser string, cmd []string) (string, string, error)
	// ExecStream writes the command's stdout as it arrives; a non-zero exit is an error. Nil execs through Docker.
	ExecStream func(ctx context.Context, containerID string, cmd []string, stdout io.Writer) error
	// DownloadKeepAlive is the gzip sync-flush interval while a streamed download restores; zero means 10 s.
	DownloadKeepAlive time.Duration
	// DownloadPrepareBudget bounds repository and password resolution and the restore of a streamed download; zero means 4 min.
	DownloadPrepareBudget time.Duration
	BackupVolumeName      string
	ZoneProvider          scheduleutil.ZoneProvider
	Location              *time.Location
	EncryptionKey         string
}
