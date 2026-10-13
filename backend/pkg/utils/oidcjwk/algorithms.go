// Source: getarcaneapp/arcane/backend/pkg/utils/oidcjwk/algorithms.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package oidcjwk

import (
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/lestrrat-go/jwx/v4/jwa"
)

func Algorithms() []string {
	return []string{
		jwa.MLDSA44().String(),
		jwa.MLDSA65().String(),
		jwa.MLDSA87().String(),
	}
}

func SupportedSigningAlgs() []string {
	return append([]string{
		oidc.RS256, oidc.RS384, oidc.RS512,
		oidc.ES256, oidc.ES384, oidc.ES512,
		oidc.PS256, oidc.PS384, oidc.PS512,
		oidc.EdDSA,
	}, Algorithms()...)
}
