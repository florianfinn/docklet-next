// Source: getarcaneapp/arcane/backend/internal/container/children/stats/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package stats

import (
	"testing"

	"github.com/florianfinn/docklet-next/types/container"
	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
)

func TestResourceSortPermissionCheck(t *testing.T) {
	listOnly := authz.NewPermissionSet()
	listOnly.AddEnv("env-1", authz.PermContainersList)
	full := authz.NewPermissionSet()
	full.AddEnv("env-1", authz.PermContainersList, authz.PermContainersRead)

	require.True(t, ContainerResourceSortPermissionDenied(listOnly, "env-1", container.SortCPUUsage))
	require.True(t, ContainerResourceSortPermissionDenied(listOnly, "env-1", container.SortMemoryUsage))
	require.False(t, ContainerResourceSortPermissionDenied(full, "env-1", container.SortMemoryUsage))
	require.False(t, ContainerResourceSortPermissionDenied(listOnly, "env-1", "name"),
		"non-resource sorts keep the endpoint's containers:list requirement")
	require.False(t, ContainerResourceSortPermissionDenied(authz.SudoPermissionSet(), "env-1", container.SortCPUUsage))
}

func TestResourceSortComparatorKeepsUnavailableLast(t *testing.T) {
	asc := ContainerResourceSampleSort(container.SortMemoryUsage, false)
	desc := ContainerResourceSampleSort(container.SortMemoryUsage, true)
	sampled := container.Summary{ID: "b", Names: []string{"b"}, ResourceSample: &container.ResourceSample{MemoryUsageBytes: 1}}
	unsampled := container.Summary{ID: "a", Names: []string{"a"}}

	require.Equal(t, 1, asc(unsampled, sampled))
	require.Equal(t, -1, asc(sampled, unsampled))
	require.Equal(t, 1, desc(unsampled, sampled))
	require.Equal(t, -1, desc(sampled, unsampled))

	zero := container.Summary{ID: "z", Names: []string{"z"}, ResourceSample: &container.ResourceSample{MemoryUsageBytes: 0}}
	require.Equal(t, -1, asc(zero, unsampled), "zero is a valid value, not unavailable")
	require.Equal(t, -1, asc(zero, sampled))
}
