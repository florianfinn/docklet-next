// Source: italypaleale/francis/components/testing/suite.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"bytes"
	"github.com/italypaleale/francis/components"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func (s Suite) TestBackupRestore(t *testing.T) {
	t.Run("wipes and reloads all persistent data", func(t *testing.T) {
		ctx := t.Context()
		require.NoError(t, s.p.Seed(ctx, Spec{}))

		// Seed one of each kind of persistent record, leaving no host connected
		SeedBackupSample(t, ctx, s.p, s.p.Now())

		// The snapshot must contain every category
		var bufA bytes.Buffer
		err := s.p.Backup(ctx, &bufA)
		require.NoError(t, err)

		setA := DecodeBackup(t, bufA.Bytes())
		require.NotEmpty(t, setA.States, "expected actor state in the backup")
		require.GreaterOrEqual(t, len(setA.Alarms), 2, "expected a plain alarm and a live job in the backup")
		require.NotEmpty(t, setA.TerminalJobs, "expected a terminal job in the backup")

		// Add records that are absent from the snapshot, so a correct restore must remove them
		AddExtraBackupData(t, ctx, s.p, s.p.Now())

		// Restore wipes the extra records and reloads the snapshot
		err = s.p.Restore(ctx, bytes.NewReader(bufA.Bytes()))
		require.NoError(t, err)

		// A fresh backup must reproduce exactly the snapshot, proving both the wipe and the load
		var bufB bytes.Buffer
		err = s.p.Backup(ctx, &bufB)
		require.NoError(t, err)

		setB := DecodeBackup(t, bufB.Bytes())

		AssertBackupContentsEqual(t, setA, setB)
	})

	t.Run("backup runs online but restore refuses while a host is connected", func(t *testing.T) {
		ctx := t.Context()

		err := s.p.Seed(ctx, Spec{
			Hosts: HostSpecCollection{
				{HostID: SpecHostH1, Address: "127.0.0.1:4001", LastHealthAgo: time.Second},
			},
		})
		require.NoError(t, err)

		// Backup takes a consistent snapshot without requiring a quiescent cluster
		var buf bytes.Buffer
		err = s.p.Backup(ctx, &buf)
		require.NoError(t, err)

		// Restore would corrupt running actors, so it refuses while a host is connected
		err = s.p.Restore(ctx, bytes.NewReader(buf.Bytes()))
		require.ErrorIs(t, err, components.ErrHostsConnected)
	})
}
