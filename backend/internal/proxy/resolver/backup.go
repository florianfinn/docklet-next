// Source: docklet-next/issues/492
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original resolver execution, recovery and verification.

package resolver

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/google/uuid"
)

func randomID() string { return uuid.NewString() }

type Target struct {
	Source     dto.Source `json:"source"`
	ProjectID  string     `json:"projectId"`
	Directory  string     `json:"directory"`
	Compose    string     `json:"compose"`
	Project    string     `json:"project"`
	Service    string     `json:"service"`
	Static     string     `json:"static,omitempty"`
	AutoInject bool       `json:"autoInject"`
	Files      []File     `json:"files"`
}

func syncDirectory(p string) error {
	f, e := os.Open(p)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}
func exclusive(p string, b []byte) error {
	f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer f.Close()
	if _, e = f.Write(b); e != nil {
		return e
	}
	return f.Sync()
}
func secureDirectory(p string) error {
	if e := os.MkdirAll(p, 0700); e != nil {
		return e
	}
	fd, _, e := parent(p, "placeholder")
	if e != nil {
		return e
	}
	os.NewFile(uintptr(fd), p).Close()
	st, e := os.Lstat(p)
	if e != nil {
		return e
	}
	if !st.IsDir() || st.Mode().Perm() != 0700 {
		return dto.Fail(dto.BackupInvalid, 409)
	}
	return nil
}
func Publish(root, id string, t Target) error {
	if !relative(id) || filepath.Base(id) != id {
		return dto.Fail(dto.InvalidInput, 422)
	}
	if e := secureDirectory(root); e != nil {
		return e
	}
	dir := filepath.Join(root, id)
	if e := os.Mkdir(dir, 0700); e != nil {
		return e
	}
	for i, f := range t.Files {
		if f.Old.Present {
			if e := exclusive(filepath.Join(dir, strconv.Itoa(i)), f.Before); e != nil {
				return e
			}
		}
	}
	b, e := json.Marshal(t)
	if e != nil {
		return e
	}
	if e = exclusive(filepath.Join(dir, "manifest.json"), b); e != nil {
		return e
	}
	if e = syncDirectory(dir); e != nil {
		return e
	}
	return syncDirectory(root)
}
func Load(root, id string, files []dto.ResolverFile) (Target, error) {
	var t Target
	fail := func() (Target, error) { return Target{}, dto.Fail(dto.BackupInvalid, 409) }
	if !relative(id) || filepath.Base(id) != id {
		return fail()
	}
	dir := filepath.Join(root, id)
	st, e := os.Lstat(dir)
	if e != nil || !st.IsDir() || st.Mode().Perm() != 0700 {
		return fail()
	}
	fd, _, e := parent(dir, "manifest.json")
	if e != nil {
		return fail()
	}
	_ = os.NewFile(uintptr(fd), dir).Close()
	m, e := Read(dir, "manifest.json")
	if e != nil || !m.Old.Present || m.Mode != 0600 {
		return fail()
	}
	if json.Unmarshal(m.Before, &t) != nil || !filepath.IsAbs(t.Directory) || !relative(t.Compose) || t.ProjectID == "" || t.Service == "" || len(t.Files) != len(files) || len(files) == 0 {
		return fail()
	}
	seen := map[string]bool{}
	for i := range t.Files {
		f := &t.Files[i]
		if !relative(f.Path) || seen[f.Path] || f.ResolverFile != files[i] || f.Mode&07000 != 0 || f.Mode&0022 != 0 {
			return fail()
		}
		seen[f.Path] = true
		if f.Old.Present {
			b, e := Read(dir, strconv.Itoa(i))
			if e != nil || b.Mode != 0600 || snapshot(b.Before, true) != f.Old {
				return fail()
			}
			f.Before = b.Before
		}
	}
	return t, nil
}
func Remove(root, id string) error {
	if !relative(id) || filepath.Base(id) != id {
		return fmt.Errorf("invalid backup identifier")
	}
	// OpenRoot confines cleanup even if an operator replaced a backup by a symlink.
	r, e := os.OpenRoot(root)
	if errors.Is(e, os.ErrNotExist) {
		return nil
	}
	if e != nil {
		return e
	}
	defer r.Close()
	if e = r.RemoveAll(id); e != nil {
		return e
	}
	return syncDirectory(root)
}

// Current is a stable confirmation of every touched path and current file hash.
// It must not apply the writer's metadata rules, so Release stays possible.
func Current(t Target) (dto.FileState, error) {
	type entry struct {
		Path  string        `json:"path"`
		State dto.FileState `json:"state"`
	}
	states := make([]entry, 0, len(t.Files))
	for _, f := range t.Files {
		state, e := observe(t.Directory, f.Path)
		if e != nil {
			return dto.FileState{}, dto.Fail(dto.AgentUnavailable, 503)
		}
		states = append(states, entry{f.Path, state})
	}
	b, e := json.Marshal(states)
	if e != nil {
		return dto.FileState{}, e
	}
	return snapshot(b, true), nil
}
