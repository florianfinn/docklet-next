// Source: getarcaneapp/kit/updater/internal/compose/compose_test.go
// Revision: 119f68956cee6d5aba8a5463561aaf9be9943b65
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package compose

import "testing"

func TestComposeLabels(t *testing.T) {
	labels := map[string]string{
		ProjectLabelKey:     " app ",
		ServiceLabelKey:     "web",
		WorkingDirLabelKey:  " /srv/app ",
		ConfigFilesLabelKey: " compose.yaml, compose.override.yaml ,, ",
	}
	if got := ProjectLabel(labels); got != "app" {
		t.Fatalf("ProjectLabel() = %q, want app", got)
	}
	if got := ServiceLabel(labels); got != "web" {
		t.Fatalf("ServiceLabel() = %q, want web", got)
	}
	if got := WorkingDirLabel(labels); got != "/srv/app" {
		t.Fatalf("WorkingDirLabel() = %q, want /srv/app", got)
	}
	files := ConfigFilesLabel(labels)
	if len(files) != 2 || files[0] != "compose.yaml" || files[1] != "compose.override.yaml" {
		t.Fatalf("ConfigFilesLabel() = %#v, want two config files", files)
	}
}
