// Source: docklet-next/issues/663
// Revision: 723c0de3cd289f18b8ddb41e919096e8d3479536
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original descriptor-based metadata loss comparison.

package hostio

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"golang.org/x/sys/unix"
)

// LossError keeps raw attribute values local while carrying the public loss receipt.
type LossError struct {
	Problem *dto.Problem
	Loss    dto.MetadataLoss
}

func (e *LossError) Error() string { return e.Problem.Error() }
func (e *LossError) Unwrap() error { return e.Problem }
func RefuseLoss(l dto.MetadataLoss) error {
	return &LossError{dto.Fail(dto.MetadataLossReason, 409), l}
}
func HasLoss(l dto.MetadataLoss) bool { return l.Links != 0 || len(l.Attributes) != 0 }
func Acknowledged(l dto.MetadataLoss, ack []dto.MetadataLoss) bool {
	if !HasLoss(l) {
		return true
	}
	for _, a := range ack {
		if a.Path == l.Path && a.Token == l.Token {
			return true
		}
	}
	return false
}

type attributeReader struct {
	list func(int, []byte) (int, error)
	get  func(int, string, []byte) (int, error)
}

func unsupported() error        { return dto.Fail(dto.UnsupportedMetadata, 409) }
func noAttributes(e error) bool { return errors.Is(e, unix.ENOTSUP) || errors.Is(e, unix.EOPNOTSUPP) }
func sized(read func([]byte) (int, error), limit int) ([]byte, error) {
	for attempt := 0; attempt < 2; attempt++ {
		n, e := read(nil)
		if noAttributes(e) {
			return nil, nil
		}
		if e != nil {
			return nil, unsupported()
		}
		if n < 0 || n > limit {
			return nil, unsupported()
		}
		if n == 0 {
			return nil, nil
		}
		b := make([]byte, n)
		m, e := read(b)
		if errors.Is(e, unix.ERANGE) || e == nil && m != n {
			continue
		}
		if e != nil || m < 0 || m > len(b) {
			return nil, unsupported()
		}
		return b[:m], nil
	}
	return nil, unsupported()
}
func (r attributeReader) attributes(fd int) (map[string][]byte, error) {
	names, e := sized(func(b []byte) (int, error) { return r.list(fd, b) }, 65536)
	if e != nil {
		return nil, e
	}
	out := map[string][]byte{}
	total := len(names)
	if len(names) > 0 && names[len(names)-1] != 0 {
		return nil, unsupported()
	}
	for _, name := range strings.Split(strings.TrimSuffix(string(names), "\x00"), "\x00") {
		if name == "" {
			if len(names) == 0 {
				break
			}
			return nil, unsupported()
		}
		if len(name) > 255 || len(out) >= 64 {
			return nil, unsupported()
		}
		for _, c := range name {
			if c < 33 || c > 126 {
				return nil, unsupported()
			}
		}
		if _, ok := out[name]; ok {
			return nil, unsupported()
		}
		b, e := sized(func(b []byte) (int, error) { return r.get(fd, name, b) }, 65536-total)
		if e != nil {
			return nil, e
		}
		total += len(b)
		out[name] = b
	}
	return out, nil
}

// acl normalises the Linux v2 ACL. Mode-controlled entries are validated but
// excluded from comparison; named entries and the owning group remain significant.
func acl(b []byte) ([]byte, error) {
	if len(b) < 4 || binary.LittleEndian.Uint32(b) != 2 || (len(b)-4)%8 != 0 {
		return nil, unsupported()
	}
	seen := map[uint64]bool{}
	entries := [][]byte{}
	base := map[uint16]bool{}
	named := false
	for i := 4; i < len(b); i += 8 {
		tag := binary.LittleEndian.Uint16(b[i:])
		perm := binary.LittleEndian.Uint16(b[i+2:])
		id := binary.LittleEndian.Uint32(b[i+4:])
		key := uint64(tag)<<32 | uint64(id)
		if perm > 7 || seen[key] {
			return nil, unsupported()
		}
		seen[key] = true
		switch tag {
		case 1, 4, 16, 32:
			if id != ^uint32(0) || base[tag] {
				return nil, unsupported()
			}
			base[tag] = true
		case 2, 8:
			if id == ^uint32(0) {
				return nil, unsupported()
			}
			named = true
		default:
			return nil, unsupported()
		}
		if tag == 2 || tag == 4 || tag == 8 {
			entries = append(entries, append([]byte(nil), b[i:i+8]...))
		}
	}
	if !base[1] || !base[4] || !base[32] || named && !base[16] {
		return nil, unsupported()
	}
	sort.Slice(entries, func(i, j int) bool { return bytes.Compare(entries[i], entries[j]) < 0 })
	return bytes.Join(entries, nil), nil
}

// CompareMetadata compares pinned regular files after the caller's final chmod.
// A negative target descriptor denotes a new target, which cannot lose metadata.
func CompareMetadata(target, staged int, p string) (dto.MetadataLoss, error) {
	return compareMetadata(target, staged, p, attributeReader{unix.Flistxattr, unix.Fgetxattr})
}
func compareMetadata(target, staged int, p string, r attributeReader) (dto.MetadataLoss, error) {
	loss := dto.MetadataLoss{Path: p}
	if target < 0 {
		return loss, nil
	}
	var st unix.Stat_t
	if e := unix.Fstat(target, &st); e != nil {
		return loss, e
	}
	if st.Nlink > 1025 {
		return loss, unsupported()
	}
	old, e := r.attributes(target)
	if e != nil {
		return loss, e
	}
	next, e := r.attributes(staged)
	if e != nil {
		return loss, e
	}
	raw := map[string][]byte{}
	for n, v := range old {
		raw[n] = v
	}
	for _, attrs := range []map[string][]byte{old, next} {
		if b, ok := attrs["system.posix_acl_access"]; ok {
			c, e := acl(b)
			if e != nil {
				return loss, e
			}
			attrs["system.posix_acl_access"] = c
		}
	}
	names := []string{}
	for n, v := range old {
		if b, ok := next[n]; !ok || !bytes.Equal(v, b) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	loss.Attributes = names
	loss.Links = int(st.Nlink) - 1
	if HasLoss(loss) {
		type pair struct {
			Name  string
			Value []byte
		}
		pairs := []pair{}
		for _, n := range names {
			pairs = append(pairs, pair{n, raw[n]})
		}
		b, _ := json.Marshal(struct {
			Attributes        []pair
			Links, Dev, Inode uint64
		}{pairs, uint64(st.Nlink), uint64(st.Dev), st.Ino})
		sum := sha256.Sum256(b)
		loss.Token = hex.EncodeToString(sum[:])
	}
	return loss, nil
}
