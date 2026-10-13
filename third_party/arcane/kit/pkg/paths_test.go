// Source: getarcaneapp/kit/pkg/paths_test.go
// Revision: 9c61043fc52d1977d644f075d9c168335c3a6df7
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed SQLite runtime and migrations.

package kit

import "testing"

func TestNormalizeRelativePath(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", ".", "..", "/absolute", "../escape", "a/../../b", `folder\file`, "name\x00"} {
		if _, err := NormalizeRelativePath(input); err == nil {
			t.Errorf("NormalizeRelativePath(%q) accepted invalid input", input)
		}
	}
	for _, test := range []struct{ input, want string }{
		{"folder/file.txt", "folder/file.txt"},
		{" ./a//b/../c/ ", "a/c"},
		{"folder/", "folder"},
	} {
		if got, err := NormalizeRelativePath(test.input); err != nil || got != test.want {
			t.Errorf("NormalizeRelativePath(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
}

func TestValidateFileName(t *testing.T) {
	t.Parallel()

	for _, input := range []string{"", ".", "..", "folder/name", `folder\name`, "name\x00"} {
		if _, err := ValidateFileName(input); err == nil {
			t.Errorf("ValidateFileName(%q) accepted invalid input", input)
		}
	}
	if got, err := ValidateFileName(" notes.txt "); err != nil || got != "notes.txt" {
		t.Errorf("ValidateFileName = %q, %v; want notes.txt", got, err)
	}
}

func TestFilePathMatches(t *testing.T) {
	t.Parallel()

	if !FilePathMatches("a/b", "a") || !FilePathMatches("a", "a") {
		t.Error("FilePathMatches should match the root and paths beneath it")
	}
	if FilePathMatches("ab", "a") || FilePathMatches("b/a", "a") {
		t.Error("FilePathMatches matched a sibling prefix")
	}
}

func TestSanitizeBrowsePath(t *testing.T) {
	t.Parallel()

	for _, test := range []struct{ input, want string }{
		{".", "/"},
		{"", "/"},
		{"/", "/"},
		{"a/b", "/a/b"},
		{"/a/../b", "/b"},
		{" /a/./b/ ", "/a/b"},
	} {
		if got, err := SanitizeBrowsePath(test.input); err != nil || got != test.want {
			t.Errorf("SanitizeBrowsePath(%q) = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
	for _, input := range []string{"..", "a/../../../etc", "../x"} {
		if _, err := SanitizeBrowsePath(input); err == nil {
			t.Errorf("SanitizeBrowsePath(%q) accepted traversal", input)
		}
	}
}
