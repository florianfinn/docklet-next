// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package image

import (
	"encoding/json/v2"
	"github.com/florianfinn/docklet-next/types/vulnerability"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestImageSummaryDoesNotEmbedVulnerabilityScan(t *testing.T) {
	body, err := json.Marshal(Summary{VulnerabilityScan: &vulnerability.ScanSummary{}})
	require.NoError(t, err)
	require.NotContains(t, string(body), "vulnerabilityScan")
}
