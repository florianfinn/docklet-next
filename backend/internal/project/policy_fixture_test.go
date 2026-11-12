// Source: docklet-next/issues/242
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original policy deletion models for the light project fixture.

package project

import policyModels "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"

func projectPolicyFixtureModels() []any {
	return []any{&policyModels.Policy{}, &policyModels.Child{}, &policyModels.Candidate{}, &policyModels.Notice{}}
}
