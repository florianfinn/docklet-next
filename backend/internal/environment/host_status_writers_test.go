// Source: docklet-next/issues/100
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original structural guard for the writers of the host table and its status.

package environment

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

// Every function in the backend that updates rows of the environments table. Status
// writers among them must keep pending and the update-required verdict inside the write;
// only the compatibility check (current registration) and registration/session
// transitions replace them (class table: M14 host verdict).
var hostTableWriters = []string{
	"internal/environment/agent_access.go:EnvironmentService.agentCompatibilityFailure",          // Direct credential rejection, credential-fenced
	"internal/environment/agent_access.go:EnvironmentService.runAgentCompatibility",              // compatibility verdict, credential-fenced
	"internal/environment/service.go:EnvironmentService.EnsureLocalEnvironment",                  // local api_url/credential revision only
	"internal/environment/service.go:EnvironmentService.UpdateEnvironment",                       // editable columns only (allowlist)
	"internal/environment/service.go:EnvironmentService.linkEnvironmentApiKey",                   // credential link → pending
	"internal/environment/service_health.go:EnvironmentService.UpdateEnvironmentConnectionState", // connect → pending; disconnect keeps pending/verdict
	"internal/environment/service_health.go:EnvironmentService.UpdateEnvironmentHeartbeat",       // hub id 0 only
	"internal/environment/service_health.go:EnvironmentService.updateEnvironmentStatusInternal",  // failed probe keeps pending/Edge verdict
	"internal/environment/service_runtime.go:EnvironmentService.ReconcileEdgeStatusesOnStartup",  // keeps pending/offline/verdict
}

var (
	hostWriteMethods = map[string]bool{"Update": true, "Updates": true, "UpdateColumn": true, "UpdateColumns": true, "Save": true}
	hostRawWrite     = regexp.MustCompile(`(?is)\b(update|insert\s+into|replace\s+into)\s+["\x60]?environments\b`)
)

func isEnvironmentType(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return isEnvironmentType(e.X)
	case *ast.Ident:
		return e.Name == "Environment"
	case *ast.SelectorExpr:
		return e.Sel.Name == "Environment"
	}
	return false
}

// hostTableWriterFuncs reports "file:Receiver.Func" for each function whose body updates
// the environments table through GORM (Model of an Environment value, Table("environments"),
// Save of an Environment) or raw SQL. Types are inferred syntactically: declared or
// composite-literal Environment values and results of calls named Get…Environment….
func hostTableWriterFuncs(fset *token.FileSet, file string, src *ast.File) []string {
	var found []string
	for _, decl := range src.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := fn.Name.Name
		if fn.Recv != nil && len(fn.Recv.List) == 1 {
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if id, ok := recv.(*ast.Ident); ok {
				name = id.Name + "." + name
			}
		}
		envs := map[string]bool{}
		for _, list := range []*ast.FieldList{fn.Type.Params, fn.Type.Results} {
			if list == nil {
				continue
			}
			for _, field := range list.List {
				if isEnvironmentType(field.Type) {
					for _, n := range field.Names {
						envs[n.Name] = true
					}
				}
			}
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ValueSpec:
				if x.Type != nil && isEnvironmentType(x.Type) {
					for _, id := range x.Names {
						envs[id.Name] = true
					}
				}
			case *ast.AssignStmt:
				if len(x.Rhs) == 0 || len(x.Lhs) == 0 {
					return true
				}
				id, ok := x.Lhs[0].(*ast.Ident)
				if !ok {
					return true
				}
				rhs := x.Rhs[0]
				if u, ok := rhs.(*ast.UnaryExpr); ok {
					rhs = u.X
				}
				switch r := rhs.(type) {
				case *ast.CompositeLit:
					envs[id.Name] = envs[id.Name] || isEnvironmentType(r.Type)
				case *ast.CallExpr:
					callee := ""
					if sel, ok := r.Fun.(*ast.SelectorExpr); ok {
						callee = sel.Sel.Name
					} else if fid, ok := r.Fun.(*ast.Ident); ok {
						callee = fid.Name
					}
					if strings.HasPrefix(callee, "Get") && strings.Contains(callee, "Environment") {
						envs[id.Name] = true
					}
				}
			}
			return true
		})
		queries := map[string]bool{} // variables holding a query scoped to the host table
		isEnv := func(e ast.Expr) bool {
			if u, ok := e.(*ast.UnaryExpr); ok {
				e = u.X
			}
			switch v := e.(type) {
			case *ast.CompositeLit:
				return isEnvironmentType(v.Type)
			case *ast.Ident:
				return envs[v.Name]
			}
			return false
		}
		// scoped walks a receiver chain such as db.Model(x).Where(...) or q.Where(...).
		scoped := func(chain ast.Expr) bool {
			for chain != nil {
				if id, ok := chain.(*ast.Ident); ok {
					return queries[id.Name]
				}
				inner, ok := chain.(*ast.CallExpr)
				if !ok {
					return false
				}
				innerSel, ok := inner.Fun.(*ast.SelectorExpr)
				if !ok {
					return false
				}
				if innerSel.Sel.Name == "Model" && len(inner.Args) == 1 && isEnv(inner.Args[0]) {
					return true
				}
				if innerSel.Sel.Name == "Table" && len(inner.Args) == 1 {
					if lit, ok := inner.Args[0].(*ast.BasicLit); ok && strings.Trim(lit.Value, "\"`") == "environments" {
						return true
					}
				}
				chain = innerSel.X
			}
			return false
		}
		for changed := true; changed; {
			changed = false
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if x, ok := n.(*ast.AssignStmt); ok && len(x.Lhs) == len(x.Rhs) {
					for i, lhs := range x.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && !queries[id.Name] && scoped(x.Rhs[i]) {
							queries[id.Name], changed = true, true
						}
					}
				}
				return true
			})
		}
		writes := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || writes {
				return !writes
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch {
			case sel.Sel.Name == "Exec" || sel.Sel.Name == "Raw":
				if len(call.Args) > 0 {
					if lit, ok := call.Args[0].(*ast.BasicLit); ok && lit.Kind == token.STRING {
						text, _ := strconv.Unquote(lit.Value)
						writes = hostRawWrite.MatchString(text)
					}
				}
			case hostWriteMethods[sel.Sel.Name]:
				// Save, Updates and UpdateColumns take an Environment value as their model.
				if sel.Sel.Name != "Update" && sel.Sel.Name != "UpdateColumn" && len(call.Args) > 0 && isEnv(call.Args[0]) {
					writes = true
					break
				}
				writes = scoped(sel.X)
			}
			return !writes
		})
		if writes {
			found = append(found, file+":"+name)
		}
	}
	return found
}

func TestHostTableWritersAreTheReviewedSet(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	fset := token.NewFileSet()
	var found []string
	require.NoError(t, filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || d.Name() == "vendor" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		found = append(found, hostTableWriterFuncs(fset, filepath.ToSlash(rel), src)...)
		return nil
	}))
	require.ElementsMatch(t, hostTableWriters, found, "a new writer of the host table needs a place in the reviewed writer table")
}

// The guard recognises the ordinary forms of a host table write, in any package.
func TestHostTableWriterGuardRecognisesWriteForms(t *testing.T) {
	forms := map[string]string{
		"map key":        `db.Model(&Environment{}).Where("id = ?", id).Updates(map[string]any{"Status": "pending"})`,
		"struct updates": `db.Model(&environment.Environment{}).Where("id = ?", id).Updates(Environment{Status: "pending"})`,
		"struct save":    `var env Environment; env.Status = "pending"; db.Save(&env)`,
		"update column":  `db.Model(&Environment{}).Where("id = ?", id).UpdateColumn("status", "pending")`,
		"raw sql":        "db.Exec(`UPDATE environments SET status = 'pending' WHERE id = ?`, id)",
		"constant key":   `db.Table("environments").Where("id = ?", id).Updates(map[string]any{column: "pending"})`,
		"loaded row":     `env, _ := s.GetEnvironmentByID(ctx, id); db.Model(env).Update("status", "pending")`,
		"value model":    `env := &Environment{ID: id}; db.Updates(env)`,
		"query variable": `q := db.Model(&Environment{}); q = q.Where("id = ?", id); q.Update("status", "pending")`,
	}
	for name, body := range forms {
		src := "package other\nfunc (s *Service) write() {\n" + body + "\n}\n"
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "other.go", src, parser.SkipObjectResolution)
		require.NoError(t, err, name)
		require.Equal(t, []string{"internal/other/other.go:Service.write"}, hostTableWriterFuncs(fset, "internal/other/other.go", file), name)
	}
	src := "package other\nfunc (s *Service) read() {\ndb.Model(&Environment{}).Where(\"id = ?\", id).First(&env)\ndb.Model(&Run{}).Updates(map[string]any{\"status\": \"done\"})\n}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "other.go", src, parser.SkipObjectResolution)
	require.NoError(t, err)
	require.Empty(t, hostTableWriterFuncs(fset, "internal/other/other.go", file))
}

// The generic edit sink accepts only its editable columns, so no spelling of the
// status column (GORM field name, SQLite case variants) can replace a verdict.
func TestGenericHostUpdateRefusesStatus(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Update("status", "update-required").Error)
	for _, key := range []string{"status", "Status", "STATUS", "sTaTuS", "last_seen", "is_edge"} {
		_, err := f.service.UpdateEnvironment(t.Context(), f.id, map[string]any{key: "pending"}, nil, nil)
		require.Error(t, err, key)
		row, err := f.service.GetEnvironmentByID(t.Context(), f.id)
		require.NoError(t, err)
		require.Equal(t, "update-required", row.Status, key)
	}
	_, err := f.service.UpdateEnvironment(t.Context(), f.id, map[string]any{"name": "Example"}, nil, nil)
	require.NoError(t, err)
}
