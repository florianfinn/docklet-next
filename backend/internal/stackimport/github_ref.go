// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded one-time stack import implementation.

package stackimport

import (
	dto "github.com/florianfinn/docklet-next/types/stackimport"
	"net/url"
	"regexp"
	"strings"
	"unicode"
)

var ownerPattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
var repoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}$`)

type githubRef struct {
	owner, repo, ref string
	path             []string
}

func parseGitHubRef(raw string) (githubRef, error) {
	fail := func() (githubRef, error) { return githubRef{}, dto.Failure("import.invalid-reference", 400) }
	value := raw
	isURL := strings.Contains(raw, "://")
	if isURL {
		u, err := url.Parse(raw)
		if err != nil || u.Scheme != "https" || u.Host != "github.com" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return fail()
		}
		value = strings.TrimPrefix(u.EscapedPath(), "/")
	}
	parts := strings.Split(value, "/")
	for i, p := range parts {
		decoded, err := url.PathUnescape(p)
		if err != nil || decoded == "" || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/\\?#%") || strings.IndexFunc(decoded, unicode.IsControl) >= 0 {
			return fail()
		}
		parts[i] = decoded
	}
	if len(parts) < 2 || (!ownerPattern.MatchString(parts[0]) || strings.Contains(parts[0], "--")) || !repoPattern.MatchString(parts[1]) {
		return fail()
	}
	out := githubRef{owner: parts[0], repo: parts[1], ref: "HEAD", path: parts[2:]}
	if isURL && len(parts) > 2 {
		if len(parts) < 4 || parts[2] != "tree" {
			return fail()
		}
		out.ref, out.path = parts[3], parts[4:]
	}
	return out, nil
}
func (r githubRef) relative(file string) string {
	parts := append([]string{r.owner, r.repo, r.ref}, r.path...)
	parts = append(parts, file)
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}
func (r githubRef) canonical() string {
	parts := append([]string{r.owner, r.repo, "tree", r.ref}, r.path...)
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "https://github.com/" + strings.Join(parts, "/")
}
