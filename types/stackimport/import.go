// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original hub import contract and resolver boundary.

package stackimport

type Platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
}
type Request struct {
	Kind     string    `json:"kind" enum:"github,registry"`
	Ref      string    `json:"ref" minLength:"1" maxLength:"512"`
	Platform *Platform `json:"platform,omitempty"`
}
type Source struct {
	Kind           string `json:"kind" enum:"github,registry"`
	CanonicalRef   string `json:"canonicalRef"`
	ResolvedDigest string `json:"resolvedDigest,omitempty"`
}
type Result struct {
	Title          string   `json:"title"`
	ComposeContent string   `json:"composeContent"`
	EnvNames       []string `json:"envNames" nullable:"false"`
	Found          []string `json:"found" nullable:"false" enum:"import.found.compose,import.found.env-example,import.found.image-config,import.found.ports,import.found.volumes,import.found.environment"`
	Missing        []string `json:"missing" nullable:"false" enum:"import.missing.env-file,import.missing.label-file,import.missing.include,import.missing.extends,import.missing.build-context,import.missing.variables,import.missing.compose"`
	Source         Source   `json:"source"`
}
type Problem struct {
	Code   string `json:"code" enum:"import.invalid-reference,import.registry-not-configured,import.not-found,import.too-large,import.timeout,import.redirect-refused,import.source-unavailable,import.invalid-compose,import.unauthorized,import.busy"`
	Status int    `json:"status"`
	Line   int    `json:"line,omitempty"`
}

func (p *Problem) Error() string               { return p.Code }
func (p *Problem) GetStatus() int              { return p.Status }
func Failure(code string, status int) *Problem { return &Problem{Code: code, Status: status} }
