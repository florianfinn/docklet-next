// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original loss-rule regression proofs.

package hostio

import (
	"encoding/binary"
	"fmt"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
	"testing"
)

func testACL(id uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, 2)
	for _, v := range [][3]uint32{{1, 7, ^uint32(0)}, {2, 4, id}, {4, 5, ^uint32(0)}, {16, 5, ^uint32(0)}, {32, 0, ^uint32(0)}} {
		b = binary.LittleEndian.AppendUint16(b, uint16(v[0]))
		b = binary.LittleEndian.AppendUint16(b, uint16(v[1]))
		b = binary.LittleEndian.AppendUint32(b, v[2])
	}
	return b
}
func lossFixture(t *testing.T, defaultACL bool) (ProjectWriter, string) {
	t.Helper()
	root := t.TempDir()
	if defaultACL {
		require.NoError(t, unix.Setxattr(root, "system.posix_acl_default", testACL(12345), 0))
	}
	w := ProjectWriter{Root: root}
	require.NoError(t, w.Write(t.Context(), "target", []byte("old"), 0644))
	require.NoError(t, w.Write(t.Context(), "stage", []byte("new"), 0644))
	return w, root
}
func TestM44InheritedACLAndMode(t *testing.T) {
	w, root := lossFixture(t, true)
	for _, mode := range []os.FileMode{0644, 0640} {
		require.NoError(t, os.Chmod(filepath.Join(root, "target"), mode))
		l, e := w.Prepare(t.Context(), "stage", "target")
		require.NoError(t, e)
		require.False(t, HasLoss(l), "mode mask changes are not ACL loss")
		st, err := os.Stat(filepath.Join(root, "stage"))
		require.NoError(t, err)
		require.Equal(t, mode, st.Mode().Perm())
	}
	require.NoError(t, w.Replace(t.Context(), "stage", "target"))
	require.NoError(t, w.Remove(t.Context(), "target"))
}
func TestM44LossAndValueConfirmation(t *testing.T) {
	for _, kind := range []string{"acl", "attribute", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			w, root := lossFixture(t, true)
			target := filepath.Join(root, "target")
			switch kind {
			case "acl":
				require.NoError(t, unix.Setxattr(target, "system.posix_acl_access", testACL(12346), 0))
			case "attribute":
				require.NoError(t, unix.Setxattr(target, "user.example", []byte("one"), 0))
			case "hardlink":
				require.NoError(t, os.Link(target, filepath.Join(root, "other")))
			}
			l, e := w.Prepare(t.Context(), "stage", "target")
			require.NoError(t, e)
			require.True(t, HasLoss(l))
			require.Len(t, l.Token, 64)
			require.ErrorContains(t, w.Replace(t.Context(), "stage", "target"), "metadata-loss")
			w.Acknowledgements = []dto.MetadataLoss{l}
			if kind == "attribute" {
				require.NoError(t, unix.Setxattr(target, "user.example", []byte("two"), 0))
				require.ErrorContains(t, w.Replace(t.Context(), "stage", "target"), "metadata-loss")
				l, e = w.Prepare(t.Context(), "stage", "target")
				require.NoError(t, e)
				w.Acknowledgements = []dto.MetadataLoss{l}
			}
			require.NoError(t, w.Replace(t.Context(), "stage", "target"))
			if kind == "hardlink" {
				b, e := os.ReadFile(filepath.Join(root, "other"))
				require.NoError(t, e)
				require.Equal(t, "old", string(b))
			}
		})
	}
}
func TestM44EntrySwap(t *testing.T) {
	for _, name := range []string{"target", "stage"} {
		t.Run(name, func(t *testing.T) {
			w, root := lossFixture(t, false)
			w.beforeRename = func() {
				require.NoError(t, os.Rename(filepath.Join(root, name), filepath.Join(root, "saved")))
				require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("foreign"), 0644))
			}
			// A swap without any loss is a changed target; it never yields a loss entry without a token.
			require.ErrorContains(t, w.Replace(t.Context(), "stage", "target"), "target-changed")
			b, e := os.ReadFile(filepath.Join(root, "target"))
			require.NoError(t, e)
			require.NotEqual(t, "new", string(b))
		})
	}
}
func TestM44AttributeReaderBounds(t *testing.T) {
	w, root := lossFixture(t, false)
	_ = w
	f, e := os.Open(filepath.Join(root, "target"))
	require.NoError(t, e)
	defer f.Close()
	for _, err := range []error{unix.ENOTSUP, unix.EACCES, unix.EIO, unix.ERANGE} {
		t.Run(err.Error(), func(t *testing.T) {
			r := attributeReader{list: func(int, []byte) (int, error) { return 0, err }}
			_, e := compareMetadata(int(f.Fd()), int(f.Fd()), "target", r)
			if noAttributes(err) {
				require.NoError(t, e)
			} else {
				require.ErrorContains(t, e, "unsupported-metadata")
			}
		})
	}
	for _, n := range []int{65537, 4} {
		r := attributeReader{list: func(_ int, b []byte) (int, error) {
			if b == nil {
				return n, nil
			}
			return 0, unix.ERANGE
		}}
		_, e := r.attributes(int(f.Fd()))
		require.ErrorContains(t, e, "unsupported-metadata")
	}
	for _, names := range []string{string(make([]byte, 4)), "a\x00" + string(make([]byte, 65535))} {
		r := attributeReader{list: func(_ int, b []byte) (int, error) {
			if b == nil {
				return len(names), nil
			}
			copy(b, names)
			return len(names), nil
		}}
		_, e := r.attributes(int(f.Fd()))
		require.ErrorContains(t, e, "unsupported-metadata")
	}
}
func TestM44SecurityLabelAndMalformedACL(t *testing.T) {
	_, root := lossFixture(t, false)
	f, e := os.Open(filepath.Join(root, "target"))
	require.NoError(t, e)
	defer f.Close()
	g, e := os.Open(filepath.Join(root, "stage"))
	require.NoError(t, e)
	defer g.Close()
	for _, equal := range []bool{true, false} {
		r := attributeReader{list: func(_ int, b []byte) (int, error) { return copyOrSize(b, []byte("security.selinux\x00")), nil }, get: func(fd int, _ string, b []byte) (int, error) {
			v := "one"
			if !equal && fd == int(g.Fd()) {
				v = "two"
			}
			return copyOrSize(b, []byte(v)), nil
		}}
		l, e := compareMetadata(int(f.Fd()), int(g.Fd()), "target", r)
		require.NoError(t, e)
		require.Equal(t, !equal, HasLoss(l))
	}
	for _, b := range [][]byte{nil, {2, 0, 0, 0}, {1, 0, 0, 0}, append(testACL(12345), 0), testACL(12345)[:12]} {
		_, e := acl(b)
		require.ErrorContains(t, e, "unsupported-metadata")
	}
}
func copyOrSize(b, v []byte) int {
	if b == nil {
		return len(v)
	}
	return copy(b, v)
}

func TestM44AttributeReaderCountTotalAndRetry(t *testing.T) {
	names := ""
	for i := 0; i < 65; i++ {
		names += fmt.Sprintf("user.attr%d\x00", i)
	}
	r := attributeReader{list: func(_ int, b []byte) (int, error) { return copyOrSize(b, []byte(names)), nil }, get: func(int, string, []byte) (int, error) { return 0, nil }}
	_, e := r.attributes(0)
	require.ErrorContains(t, e, "unsupported-metadata")
	r = attributeReader{list: func(_ int, b []byte) (int, error) { return copyOrSize(b, []byte("user.example\x00")), nil }, get: func(int, string, []byte) (int, error) { return 65536, nil }}
	_, e = r.attributes(0)
	require.ErrorContains(t, e, "unsupported-metadata")
	calls := 0
	b, e := sized(func(b []byte) (int, error) {
		calls++
		if b == nil {
			return 4, nil
		}
		if calls == 2 {
			return 0, unix.ERANGE
		}
		return copy(b, []byte("same")), nil
	}, 64)
	require.NoError(t, e)
	require.Equal(t, "same", string(b))
	require.Equal(t, 4, calls)
	w, root := lossFixture(t, false)
	require.NoError(t, unix.Setxattr(root, "system.posix_acl_default", testACL(12345), 0))
	require.NoError(t, w.Remove(t.Context(), "stage"))
	require.NoError(t, w.Write(t.Context(), "stage", []byte("new"), 0644))
	l, e := w.Prepare(t.Context(), "stage", "target")
	require.NoError(t, e)
	require.False(t, HasLoss(l), "staging-only attributes are additions")
}
