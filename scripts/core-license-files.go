//go:build ignore

// Inspect compiled licence exceptions and package hash-bound legal evidence.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type module struct {
	Path, Version, Dir string
	Replace            *module
}
type pkg struct {
	Dir                                       string
	GoFiles, CgoFiles, CFiles, HFiles, SFiles []string
	Module                                    *module
	Error                                     any
	DepsErrors                                []any
}
type evidence struct{ Path, SHA256 string }
type record struct {
	Name, Version, LocalPath, CorrespondingSource string
	LegalFiles                                    []evidence
}
type compiled struct {
	Module  string `json:"module"`
	Version string `json:"version"`
	Path    string `json:"path"`
	SHA256  string `json:"sha256"`
}

func digest(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func run(args ...string) []byte {
	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
	out, err := cmd.Output()
	if err != nil {
		panic(err)
	}
	return out
}
func main() {
	if err := work(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func work() error {
	if len(os.Args) < 2 {
		return fmt.Errorf("expected inspect or bundle")
	}
	if os.Args[1] == "inspect" {
		args := []string{"list", "-deps", "-json=Dir,GoFiles,CgoFiles,CFiles,HFiles,SFiles,Module,Error,DepsErrors", "./backend/cmd"}
		if len(os.Args) > 2 {
			args = append(args[:2], append([]string{"-tags=" + os.Args[2]}, args[2:]...)...)
		}
		decoder := json.NewDecoder(bytes.NewReader(run(args...)))
		seen := map[string]bool{}
		out := []compiled{}
		blocked := regexp.MustCompile(`(?i)\b(?:LGPL|AGPL|GPL)\b|GNU (?:Lesser |Library |Affero )?General Public License`)
		for {
			var p pkg
			if err := decoder.Decode(&p); err == io.EOF {
				break
			} else if err != nil {
				return err
			}
			if p.Error != nil || len(p.DepsErrors) > 0 {
				return fmt.Errorf("incomplete licence graph")
			}
			if p.Module == nil {
				continue
			}
			dir := p.Module.Dir
			version := p.Module.Version
			if p.Module.Replace != nil {
				dir = p.Module.Replace.Dir
				version = p.Module.Replace.Version
			}
			for _, name := range append(append(append(append(p.GoFiles, p.CgoFiles...), p.CFiles...), p.HFiles...), p.SFiles...) {
				path := filepath.Join(p.Dir, name)
				if seen[path] {
					continue
				}
				seen[path] = true
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				comments := string(data)
				functionCount := 0
				if strings.HasSuffix(path, ".go") {
					file, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ParseComments)
					if err != nil {
						return err
					}
					ast.Inspect(file, func(node ast.Node) bool {
						switch node.(type) {
						case *ast.FuncDecl, *ast.FuncLit:
							functionCount++
						}
						return true
					})
					var b strings.Builder
					for _, group := range file.Comments {
						b.WriteString(group.Text())
					}
					comments = b.String()
				}
				if blocked.MatchString(strings.Join(strings.Fields(comments), " ")) {
					if functionCount != 0 {
						return fmt.Errorf("unassessed compiled copyleft implementation")
					}
					relative, err := filepath.Rel(dir, path)
					if err != nil {
						return err
					}
					out = append(out, compiled{p.Module.Path, version, filepath.ToSlash(relative), digest(data)})
				}
			}
		}
		sort.Slice(out, func(i, j int) bool { return out[i].Module+out[i].Path < out[j].Module+out[j].Path })
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	if os.Args[1] != "bundle" || len(os.Args) != 3 {
		return fmt.Errorf("expected bundle DESTINATION")
	}
	var inventory struct{ Packages []record }
	data, err := os.ReadFile("docs/provenance/core-import.json")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &inventory); err != nil {
		return err
	}
	modules := map[string]module{}
	decoder := json.NewDecoder(bytes.NewReader(run("list", "-m", "-json", "all")))
	for {
		var m module
		if err := decoder.Decode(&m); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		modules[m.Path] = m
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("licence bundle requires the assessed linux/amd64 target")
	}
	destination := os.Args[2]
	if err = os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	copyFile := func(source, target, expected string) error {
		data, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if expected != "" && digest(data) != expected {
			return fmt.Errorf("licence evidence changed: %s", target)
		}
		path := filepath.Join(destination, target)
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		return os.WriteFile(path, data, 0644)
	}
	for _, item := range inventory.Packages {
		m, ok := modules[item.Name]
		if !ok {
			return fmt.Errorf("licence module unavailable: %s", item.Name)
		}
		dir := m.Dir
		if m.Replace != nil {
			dir = m.Replace.Dir
		}
		for _, f := range item.LegalFiles {
			source := filepath.Join(dir, f.Path)
			if strings.HasPrefix(f.Path, "repository:") {
				source = filepath.Join("third_party/arcane", strings.TrimPrefix(f.Path, "repository:"))
			}
			if item.LocalPath == "backend" || item.LocalPath == "cli" || item.LocalPath == "types" {
				source = "LICENSES/BSD-3-Clause-Arcane.txt"
			}
			if err = copyFile(source, filepath.Join("go", item.Name, f.Path), f.SHA256); err != nil {
				return err
			}
		}
	}
	for _, p := range []string{"LICENSE", "NOTICE", "LICENSES/BSD-3-Clause-Arcane.txt", "docs/provenance/core-import.json", "docs/provenance/core-compiled-notices.json"} {
		if err = copyFile(p, p, ""); err != nil {
			return err
		}
	}
	return copyFile(filepath.Join(runtime.GOROOT(), "LICENSE"), "go/standard-library/LICENSE", "")
}
