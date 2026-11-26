// Source: docklet-next/issues/651
// Revision: 855c6a7f46fb9a7e2f4c3cc875b9f6e6d70e405d
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original regression test for content-only in-place writes.

package acfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestInPlaceWriteChmodsOnlyWhenModeDiffers(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "compose.yaml")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o674); err != nil {
		t.Fatal(err)
	}
	calls := 0
	previous := chmodFile
	chmodFile = func(f *os.File, mode os.FileMode) error { calls++; return previous(f, mode) }
	t.Cleanup(func() { chmodFile = previous })

	ctx := context.Background()
	if err := Write(ctx, root, "/compose.yaml", []byte("new content"), WriteOptions{Mode: 0o674, InPlace: true}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new content" || calls != 0 {
		t.Fatalf("content-only write: contents=%q chmod calls=%d, want new content and 0", got, calls)
	}
	if err := Write(ctx, root, "/compose.yaml", []byte("newer"), WriteOptions{Mode: 0o640, InPlace: true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(target)
	if err != nil || info.Mode().Perm() != 0o640 || calls != 1 {
		t.Fatalf("mode change: mode=%v err=%v chmod calls=%d, want 0640 and 1", info.Mode().Perm(), err, calls)
	}
}

func TestInPlaceWriteKeepsStickyAndKernelClearedSetIDWithoutChmod(t *testing.T) {
	root := t.TempDir()
	calls := 0
	previous := chmodFile
	chmodFile = func(f *os.File, mode os.FileMode) error { calls++; return previous(f, mode) }
	t.Cleanup(func() { chmodFile = previous })
	ctx := context.Background()

	sticky := filepath.Join(root, "sticky.yaml")
	if err := os.WriteFile(sticky, []byte("old"), 0o664); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sticky, 0o664|os.ModeSticky); err != nil {
		t.Fatal(err)
	}
	if err := Write(ctx, root, "/sticky.yaml", []byte("new"), WriteOptions{Mode: 0o664, InPlace: true}); err != nil || calls != 0 {
		t.Fatalf("sticky content write: err=%v chmod calls=%d, want nil and 0", err, calls)
	}

	if os.Geteuid() == 0 {
		t.Skip("root keeps set-ID bits on write (CAP_FSETID)")
	}
	setuid := filepath.Join(root, "setuid.yaml")
	if err := os.WriteFile(setuid, []byte("old"), 0o764); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(setuid, 0o764|os.ModeSetuid); err != nil {
		t.Fatal(err)
	}
	if err := Write(ctx, root, "/setuid.yaml", []byte("new"), WriteOptions{Mode: 0o764, InPlace: true}); err != nil || calls != 0 {
		t.Fatalf("set-ID content write: err=%v chmod calls=%d, want nil and 0", err, calls)
	}
	if info, err := os.Stat(setuid); err != nil || info.Mode()&os.ModeSetuid != 0 {
		t.Fatalf("set-ID bit after write: mode=%v err=%v, want cleared", info.Mode(), err)
	}
}
