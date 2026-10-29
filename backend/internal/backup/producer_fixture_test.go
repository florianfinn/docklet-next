// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original parser proofs using pinned Rustic output from synthetic data.

package backup

import (
	"encoding/json/v2"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
)

// Producer: pinned Rustic v0.11.2 /rustic, SHA-256 verified from OCI layers.
// Index: sha256:fce5854a2df9fdaa1f5999df9d99d832aa8619c79eefb3e4136da46bf912a1b0.
// Binary: c42cf1b168c9e53509769bd229337efee42112331b825cab9add5ca4494c408b.
// Recorded 2026-10-28. Only summary.command is omitted because it contains local
// generator paths; IDs, summary sizes, node names and shape are unmodified.

// Raw producer stdout SHA-256: 422de10c69b246dfce1544ef80ed29a51b02d85c4ed18142214b3be9d3e9c035.
const producerBackup = `{
  "time": "2026-10-28T12:55:18.125819631+01:00",
  "program_version": "rustic v0.11.2",
  "tree": "656c3c601aaa609bc6572fe4c036883d49d417c3a3e7c032344985ba0b442288",
  "label": "fixture",
  "paths": [
    "/"
  ],
  "hostname": "example",
  "username": "",
  "uid": 0,
  "gid": 0,
  "tags": [],
  "summary": {
    "files_new": 2,
    "files_changed": 0,
    "files_unmodified": 0,
    "total_files_processed": 2,
    "total_bytes_processed": 35,
    "dirs_new": 2,
    "dirs_changed": 0,
    "dirs_unmodified": 0,
    "total_dirs_processed": 2,
    "total_dirsize_processed": 1071,
    "data_blobs": 2,
    "tree_blobs": 2,
    "data_added": 1106,
    "data_added_packed": 696,
    "data_added_files": 35,
    "data_added_files_packed": 117,
    "data_added_trees": 1071,
    "data_added_trees_packed": 579,
    "backup_start": "2026-10-28T12:55:18.132508189+01:00",
    "backup_end": "2026-10-28T12:55:18.141758245+01:00",
    "backup_duration": 0.009250056,
    "total_duration": 0.015938614
  },
  "id": "b8ef38ae75a078ea84ad7671f1ac7b3296d3dea829cf53438d5870f1a73a393e"
}
`

// Raw producer stdout SHA-256: 387a48aa0247d168c4642a166a29e7dedae51d5eb9b90aa57c178ad54bac8a2c.
const producerSnapshots = `[
  {
    "group_key": {
      "hostname": "example",
      "label": "fixture",
      "paths": [
        "/"
      ]
    },
    "snapshots": [
      {
        "time": "2026-10-28T12:55:18.125819631+01:00",
        "program_version": "rustic v0.11.2",
        "tree": "656c3c601aaa609bc6572fe4c036883d49d417c3a3e7c032344985ba0b442288",
        "label": "fixture",
        "paths": [
          "/"
        ],
        "hostname": "example",
        "username": "",
        "uid": 0,
        "gid": 0,
        "tags": [],
        "original": "b8ef38ae75a078ea84ad7671f1ac7b3296d3dea829cf53438d5870f1a73a393e",
        "summary": {
          "files_new": 2,
          "files_changed": 0,
          "files_unmodified": 0,
          "total_files_processed": 2,
          "total_bytes_processed": 35,
          "dirs_new": 2,
          "dirs_changed": 0,
          "dirs_unmodified": 0,
          "total_dirs_processed": 2,
          "total_dirsize_processed": 1071,
          "data_blobs": 2,
          "tree_blobs": 2,
          "data_added": 1106,
          "data_added_packed": 696,
          "data_added_files": 35,
          "data_added_files_packed": 117,
          "data_added_trees": 1071,
          "data_added_trees_packed": 579,
          "backup_start": "2026-10-28T12:55:18.132508189+01:00",
          "backup_end": "2026-10-28T12:55:18.141758245+01:00",
          "backup_duration": 0.009250056,
          "total_duration": 0.015938614
        },
        "id": "b8ef38ae75a078ea84ad7671f1ac7b3296d3dea829cf53438d5870f1a73a393e"
      }
    ]
  }
]
`

// Raw producer stdout SHA-256: 17e6a5d1126dfa3d2199391240a1b7207d25b2208e3c710369ce0288886e9159.
const producerLs = `[
  "folder",
  "folder/nested.txt",
  "hello.txt"
]
`

// Raw producer stdout SHA-256: 0951a35712599f1ee1958544c2efd5a0849becf6bbfce8d6aaee346f3da5dc4c.
const producerLsLong = `drwxr-xr-x     node     node         0 28 Oct 2026 11:55 "folder"` + " " + `
-rw-r--r--     node     node        20 28 Oct 2026 11:55 "hello.txt"` + " " + `
`

func TestPinnedProducerSnapshotAndListingParsers(t *testing.T) {
	snapshot, err := decodeVolumeSnapshot(producerBackup)
	require.NoError(t, err)
	require.Len(t, snapshot.ID, 64)
	require.EqualValues(t, 35, snapshot.Size)
	listed, err := decodeSnapshotsInternal(producerSnapshots)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, snapshot.ID, listed[0].ID)
	files, err := decodeVolumeFiles(producerLs, true)
	require.NoError(t, err)
	require.Equal(t, []string{"folder", "folder/nested.txt", "hello.txt"}, files)
	marked, err := markSnapshotDirectoriesInternal([]string{"folder", "hello.txt"}, strings.TrimSpace(producerLsLong))
	require.NoError(t, err)
	require.Equal(t, []string{"folder/", "hello.txt"}, marked)
}
func TestVolumeSnapshotOutputRefusesMalformedIdentityAndSummary(t *testing.T) {
	for _, output := range []string{"", "null", "{}", `{"id":"short","summary":{"total_bytes_processed":35}}`,
		producerBackup + "{}", producerBackup[:len(producerBackup)/2]} {
		snapshot, err := decodeVolumeSnapshot(output)
		require.Error(t, err, output)
		require.Zero(t, snapshot)
		var problem *dto.Problem
		require.ErrorAs(t, err, &problem)
		require.Equal(t, "volume.backup.engine-output-invalid", problem.Code)
	}
}
func TestVolumeFileOutputRefusesNullMalformedAndOversizedBrowse(t *testing.T) {
	for _, output := range []string{"null", "{}", "[", `["ok",null]`, `[""]`, `["\u0000"]`} {
		files, err := decodeVolumeFiles(output, false)
		require.Error(t, err)
		require.Nil(t, files)
	}
	files := make([]string, dto.MaxBrowseEntries+1)
	for i := range files {
		files[i] = "file"
	}
	payload, err := json.Marshal(files)
	require.NoError(t, err)
	result, err := decodeVolumeFiles(string(payload), false)
	require.Nil(t, result)
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "volume.backup.listing-too-large", problem.Code)
	// Exactly the limit succeeds; recursive inventory has only the byte budget.
	payload, err = json.Marshal(files[:dto.MaxBrowseEntries])
	require.NoError(t, err)
	result, err = decodeVolumeFiles(string(payload), false)
	require.NoError(t, err)
	require.Len(t, result, dto.MaxBrowseEntries)
}

func TestVolumeSnapshotUnknownSizeRemainsUnknown(t *testing.T) {
	for _, output := range []string{
		strings.Replace(producerBackup, `"total_bytes_processed": 35`, `"ignored_size": 35`, 1),
		strings.Replace(producerBackup, `"total_bytes_processed": 35`, `"total_bytes_processed": -1`, 1),
	} {
		snapshot, err := decodeVolumeSnapshot(output)
		require.NoError(t, err)
		require.EqualValues(t, -1, snapshot.Size)
	}
}
