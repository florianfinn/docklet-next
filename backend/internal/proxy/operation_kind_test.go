// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver receipt consistency guards.

package proxy

import (
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestR18InconsistentResolverReceiptNeverExecutes(t *testing.T) {
	for _, cell := range []struct {
		phase dto.ResolverPhase
		state dto.State
	}{{dto.ResolverApplied, dto.Pending}, {dto.ResolverRecoveryRequired, dto.Pending}, {dto.ResolverRestoring, dto.Applied}} {
		t.Run(string(cell.phase)+"/"+string(cell.state), func(t *testing.T) {
			x, _, q := resolverFixture(t)
			en := resolverEnvelope(q)
			digest, err := en.Digest()
			require.NoError(t, err)
			o := dto.Operation{ID: q.ID, InstanceID: q.Instance.ID, Kind: dto.ResolverOperation, Generation: q.Generation, Registration: q.Registration, Digest: digest, State: cell.state, Phase: dto.Phase(cell.phase), Resolver: &dto.ResolverReceipt{Intent: en.Intent, Phase: cell.phase, Files: []dto.ResolverFile{}}}
			intent, _ := marshal(en)
			raw, _ := marshal(o)
			_, decoded, err := decodeJournal(Journal{ID: q.ID, Generation: q.Generation, Registration: q.Registration, Digest: digest, State: cell.state, Phase: dto.Phase(cell.phase), Intent: intent, Result: raw})
			require.NoError(t, err)
			require.Equal(t, dto.RecoveryRequired, decoded.State)
			require.Equal(t, dto.UnknownOperationKind, decoded.Reason)
			require.Zero(t, x.f.publications)
		})
	}
}
