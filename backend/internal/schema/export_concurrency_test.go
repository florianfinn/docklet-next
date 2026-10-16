// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Blocked CSV writer regression on the configured one-connection pool and keyset sort proof.

package schema

import (
	"bytes"
	"cmp"
	"context"
	"encoding/csv"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/vulnerability/children/query"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockedCSVWriter struct {
	bytes.Buffer
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (w *blockedCSVWriter) Write(p []byte) (int, error) {
	w.once.Do(func() { close(w.started); <-w.release })
	return w.Buffer.Write(p)
}
func TestBlockedCSVLeavesPoolAvailableForCompletion(t *testing.T) {
	db := cascadeDatabase(t)
	require.NoError(t, db.Exec("INSERT INTO vulnerability_scans(id,image_name,scan_time,status) VALUES('image','example/app:1',?,'completed')", time.Now()).Error)
	require.NoError(t, db.Transaction(func(tx *gorm.DB) error {
		for i := range 300 {
			if err := tx.Exec("INSERT INTO vulnerability_scan_items(image_id,vulnerability_id,pkg_name,severity,severity_rank) VALUES('image',?,?,'HIGH',?)", fmt.Sprintf("CVE-2026-%04d", i), fmt.Sprintf("pkg-%03d", i%10), i%5).Error; err != nil {
				return err
			}
		}
		return nil
	}))
	svc := query.NewService(db, nil, nil, nil, nil, nil, nil, nil)
	for _, sorting := range []string{"severity", "pkgName", "vulnerabilityId", "installedVersion", "fixedVersion", "imageName", ""} {
		for _, order := range []pagination.SortOrder{"asc", "desc"} {
			seq, err := svc.ExportVulnerabilities(t.Context(), "0", pagination.QueryParams{Sort: sorting, Order: order, Filters: map[string]string{}})
			require.NoError(t, err)
			var ordered []int
			ids := map[string]bool{}
			count := 0
			for row, err := range seq {
				require.NoError(t, err)
				require.False(t, ids[row.VulnerabilityID], "sort=%s order=%s duplicate=%s", sorting, order, row.VulnerabilityID)
				ids[row.VulnerabilityID] = true
				index, err := strconv.Atoi(strings.TrimPrefix(row.VulnerabilityID, "CVE-2026-"))
				require.NoError(t, err)
				ordered = append(ordered, index)
				count++
			}
			require.Equal(t, 300, count, sorting)
			expected := make([]int, 300)
			for i := range expected {
				expected[i] = i
			}
			sort.Slice(expected, func(a, b int) bool {
				i, j := expected[a], expected[b]
				comparison := 0
				switch sorting {
				case "severity":
					comparison = cmp.Compare(i%5, j%5)
				case "pkgName":
					comparison = cmp.Compare(i%10, j%10)
				case "vulnerabilityId":
					comparison = cmp.Compare(i, j)
				}
				if comparison == 0 {
					comparison = cmp.Compare(i, j)
				}
				if order == pagination.SortDesc {
					return comparison > 0
				}
				return comparison < 0
			})
			require.Equal(t, expected, ordered, "sort=%s order=%s", sorting, order)

		}
	}
	runSvc := activity.NewActivityService(db, nil)
	run, err := runSvc.StartActivity(t.Context(), activity.StartActivityRequest{Type: activitytypes.TypeProjectDeploy})
	require.NoError(t, err)
	seq, err := svc.ExportVulnerabilities(t.Context(), "0", pagination.QueryParams{Sort: "severity", Order: "desc", Filters: map[string]string{}})
	require.NoError(t, err)
	writer := &blockedCSVWriter{started: make(chan struct{}), release: make(chan struct{})}
	var once sync.Once
	release := func() { once.Do(func() { close(writer.release) }) }
	defer release()
	finished := make(chan error, 1)
	go func() { finished <- query.WriteVulnerabilitiesCSV(writer, seq) }()
	select {
	case <-writer.started:
	case <-time.After(3 * time.Second):
		t.Fatal("CSV writer never blocked")
	}
	done := make(chan error, 1)
	go func() {
		_, err := runSvc.CompleteActivity(context.Background(), run.ID, activitytypes.StatusSuccess, "done", nil)
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("blocked export retained the only connection")
	}
	release()
	require.NoError(t, <-finished)
	rows, err := csv.NewReader(bytes.NewReader(writer.Bytes())).ReadAll()
	require.NoError(t, err)
	require.Len(t, rows, 301)
}
