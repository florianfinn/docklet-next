// Source: docklet-next/issues/100
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original structural guard for the writers of the host table and its status.

package environment

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/types/version"
	"github.com/stretchr/testify/require"
)

// Every function in the backend that updates rows of the environments table, with the
// number of status references in its body (string literals naming status, Status fields,
// constants naming status). Status writers must keep pending and the update-required
// verdict inside the write; only the compatibility check (current registration) and
// registration/session transitions replace them (class table: M14 host verdict). A new
// writer or a changed status reference count inside a writer needs classification here.
var hostTableWriters = map[string]int{
	"internal/environment/agent_access.go:EnvironmentService.agentCompatibilityFailure":          1,  // Direct credential rejection, credential-fenced
	"internal/environment/agent_access.go:EnvironmentService.runAgentCompatibility":              1,  // compatibility verdict, credential-fenced
	"internal/environment/service.go:EnvironmentService.CreateEnvironment":                       2,  // new row insert (pending/offline), cannot replace a verdict
	"internal/environment/service.go:EnvironmentService.EnsureLocalEnvironment":                  1,  // local host insert and api_url/credential revision only
	"internal/environment/service.go:EnvironmentService.UpdateEnvironment":                       0,  // editable columns only (allowlist + Select)
	"internal/environment/service.go:EnvironmentService.linkEnvironmentApiKey":                   3,  // credential link → pending
	"internal/environment/service_health.go:EnvironmentService.UpdateEnvironmentConnectionState": 3,  // connect → pending; disconnect keeps pending/verdict
	"internal/environment/service_health.go:EnvironmentService.UpdateEnvironmentHeartbeat":       1,  // hub id 0 only
	"internal/environment/service_health.go:EnvironmentService.updateEnvironmentStatusInternal":  10, // failed probe keeps pending/Edge verdict
	"internal/environment/service_runtime.go:EnvironmentService.ReconcileEdgeStatusesOnStartup":  4,  // keeps pending/offline/verdict
}

var (
	hostWriteMethods = map[string]bool{"Update": true, "Updates": true, "UpdateColumn": true, "UpdateColumns": true, "Save": true, "Create": true}
	// SQLite statements that change rows (UPDATE and INSERT with an optional OR conflict
	// clause, REPLACE INTO) and their target: an identifier, optionally schema-qualified,
	// unquoted or in any SQLite identifier quote.
	hostSQLIdent   = `(?:"[^"]*"|\x60[^\x60]*\x60|\[[^\]]*\]|[\w$]+)`
	hostRawWrite   = regexp.MustCompile(`(?is)\b(?:update(?:\s+or\s+\w+)?|insert(?:\s+or\s+\w+)?\s+into|replace\s+into)\s+(` + hostSQLIdent + `(?:\s*\.\s*` + hostSQLIdent + `)?)`)
	hostTableRef   = regexp.MustCompile(`^\s*(` + hostSQLIdent + `)(?:\s*\.\s*(` + hostSQLIdent + `))?`)
	hostSQLComment = regexp.MustCompile(`(?s)/\*.*?\*/|--[^\n]*`)
	hostStatusWord = regexp.MustCompile(`(?i)\bstatus\b`)
)

// isModelType reports whether a type expression names a host model: Environment or a
// type whose TableName is "environments", possibly behind a pointer, slice or array.
func (s *hostWriterScan) isModelType(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.StarExpr:
		return s.isModelType(e.X)
	case *ast.ArrayType:
		return s.isModelType(e.Elt)
	case *ast.Ident:
		return s.models[e.Name]
	case *ast.SelectorExpr:
		return s.models[e.Sel.Name]
	}
	return false
}

// isModel is isModelType for a type the checker resolved inside the scanned package.
func (s *hostWriterScan) isModel(t types.Type) bool {
	for {
		switch v := types.Unalias(t).(type) {
		case *types.Pointer:
			t = v.Elem()
		case *types.Slice:
			t = v.Elem()
		case *types.Array:
			t = v.Elem()
		case *types.Named:
			return s.models[v.Obj().Name()]
		default:
			return false
		}
	}
}

// unwrap strips parentheses, address-of and dereference around a local expression.
func unwrap(e ast.Expr) ast.Expr {
	for {
		switch v := e.(type) {
		case *ast.ParenExpr:
			e = v.X
		case *ast.StarExpr:
			e = v.X
		case *ast.UnaryExpr:
			if v.Op != token.AND {
				return e
			}
			e = v.X
		default:
			return e
		}
	}
}

func calleeName(call *ast.CallExpr) string {
	switch f := call.Fun.(type) {
	case *ast.SelectorExpr:
		return f.Sel.Name
	case *ast.Ident:
		return f.Name
	}
	return ""
}

// hostWriterScan holds what one function body binds: Environment values and query
// variables scoped to the host table. Inference is syntactic and per function;
// constant values come from go/types.
type hostWriterScan struct {
	info    *types.Info
	models  map[string]bool // host model type names of the package
	rawSQL  bool            // the current body holds a constant host table write statement
	envs    map[string]bool
	queries map[string]bool   // query holders: variables, local fields ("h.q"), elements ("qs[]")
	funcs   map[string]string // local method values of write methods: "scoped" or the method
}

// hostHolder names where a local expression stores a value: a variable, a field path, or
// the elements of a map, slice or array.
func hostHolder(e ast.Expr) string {
	e = unwrap(e)
	if index, ok := e.(*ast.IndexExpr); ok {
		return types.ExprString(unwrap(index.X)) + "[]"
	}
	return types.ExprString(e)
}

// text returns the value of a constant string expression: literals, concatenations and
// constants of the package at any scope, as the type checker evaluates them.
func (s *hostWriterScan) text(e ast.Expr) (string, bool) {
	if tv, ok := s.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return constant.StringVal(tv.Value), true
	}
	return "", false
}

// hostTableName returns the bare, lower-case table of a GORM Table argument or SQL target:
// without comments, alias, schema or SQLite identifier quotes.
func hostTableName(name string) string {
	m := hostTableRef.FindStringSubmatch(hostSQLComment.ReplaceAllString(name, " "))
	if m == nil {
		return ""
	}
	table := m[1]
	if m[2] != "" {
		table = m[2]
	}
	return strings.ToLower(strings.Trim(table, "\"`[]"))
}

// hostRawWrites reports whether constant SQL updates, inserts into or replaces into the
// environments table, ignoring SQLite comments.
func hostRawWrites(sql string) bool {
	for _, m := range hostRawWrite.FindAllStringSubmatch(hostSQLComment.ReplaceAllString(sql, " "), -1) {
		if hostTableName(m[1]) == "environments" {
			return true
		}
	}
	return false
}

// hostStubImporter gives every import an empty package, so a package type-checks without
// its dependencies; constants of the package itself still evaluate.
type hostStubImporter struct{}

func (hostStubImporter) Import(path string) (*types.Package, error) {
	pkg := types.NewPackage(path, filepath.Base(path))
	pkg.MarkComplete()
	return pkg, nil
}

// hostModels returns Environment, its type aliases and every type of the package whose
// TableName method returns the constant "environments".
func hostModels(files []*ast.File, info *types.Info) map[string]bool {
	models := map[string]bool{"Environment": true}
	scan := &hostWriterScan{info: info, models: models}
	// Type aliases of a model at any scope name a model too (syntactic fallback where
	// stub imports leave the checker without the aliased type).
	for changed := true; changed; {
		changed = false
		for _, file := range files {
			ast.Inspect(file, func(n ast.Node) bool {
				if spec, ok := n.(*ast.TypeSpec); ok && spec.Assign.IsValid() && !models[spec.Name.Name] && scan.isModelType(spec.Type) {
					models[spec.Name.Name], changed = true, true
				}
				return true
			})
		}
	}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "TableName" || fn.Recv == nil || len(fn.Recv.List) != 1 || fn.Body == nil {
				continue
			}
			recv := unwrap(fn.Recv.List[0].Type)
			id, ok := recv.(*ast.Ident)
			if !ok {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				if ret, ok := n.(*ast.ReturnStmt); ok && len(ret.Results) == 1 {
					if name, ok := scan.text(ret.Results[0]); ok && hostTableName(name) == "environments" {
						models[id.Name] = true
					}
				}
				return true
			})
		}
	}
	return models
}

// hostConstantInfo type-checks one package with stub imports, ignoring type errors.
func hostConstantInfo(fset *token.FileSet, files []*ast.File) *types.Info {
	info := &types.Info{Types: map[ast.Expr]types.TypeAndValue{}, Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
	conf := types.Config{Importer: hostStubImporter{}, Error: func(error) {}}
	_, _ = conf.Check(files[0].Name.Name, fset, files, info)
	return info
}

func (s *hostWriterScan) isEnv(e ast.Expr) bool {
	if t := s.info.TypeOf(unwrap(e)); t != nil && s.isModel(t) {
		return true
	}
	switch v := unwrap(e).(type) {
	case *ast.CompositeLit:
		return s.isModelType(v.Type)
	case *ast.Ident:
		return s.envs[v.Name]
	case *ast.IndexExpr:
		return s.isEnv(v.X)
	case *ast.CallExpr:
		name := calleeName(v)
		if name == "new" && len(v.Args) == 1 {
			return s.isModelType(v.Args[0])
		}
		return strings.HasPrefix(name, "Get") && strings.Contains(name, "Environment")
	}
	return false
}

// scoped walks a receiver chain such as db.Model(x).Where(...) or q.Where(...).
func (s *hostWriterScan) scoped(chain ast.Expr) bool {
	for chain != nil {
		chain = unwrap(chain)
		switch chain.(type) {
		case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr:
			return s.queries[hostHolder(chain)]
		}
		call, ok := chain.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if sel.Sel.Name == "Model" && len(call.Args) == 1 && s.isEnv(call.Args[0]) {
			return true
		}
		if sel.Sel.Name == "Table" && len(call.Args) > 0 {
			if name, ok := s.text(call.Args[0]); ok && hostTableName(name) == "environments" {
				return true
			}
		}
		chain = sel.X
	}
	return false
}

func (s *hostWriterScan) bind(lhs, rhs ast.Expr) bool {
	key := hostHolder(lhs)
	if key == "_" {
		return false
	}
	if id, ok := unwrap(lhs).(*ast.Ident); ok && !s.envs[id.Name] && s.isEnv(rhs) {
		s.envs[id.Name] = true
		return true
	}
	if !s.queries[key] && s.scoped(rhs) {
		s.queries[key] = true
		return true
	}
	changed := false
	switch r := unwrap(rhs).(type) {
	case *ast.SelectorExpr: // method value: write := q.Update, save := db.Save
		if _, done := s.funcs[key]; !done && (hostWriteMethods[r.Sel.Name] || r.Sel.Name == "Exec" || r.Sel.Name == "Raw") {
			s.funcs[key] = r.Sel.Name
			if hostWriteMethods[r.Sel.Name] && s.scoped(r.X) {
				s.funcs[key] = "scoped"
			}
			changed = true
		}
	case *ast.CompositeLit: // holder{q: query} or map[string]*gorm.DB{"host": query}
		for _, elt := range r.Elts {
			holder, value := key+"[]", elt
			if kv, ok := elt.(*ast.KeyValueExpr); ok {
				value = kv.Value
				if field, ok := kv.Key.(*ast.Ident); ok && !isMapOrSlice(r.Type) {
					holder = key + "." + field.Name
				}
			}
			if !s.queries[holder] && s.scoped(value) {
				s.queries[holder], changed = true, true
			}
		}
	}
	return changed
}

func isMapOrSlice(t ast.Expr) bool {
	switch t.(type) {
	case *ast.MapType, *ast.ArrayType:
		return true
	}
	return false
}

// writes reports whether body updates the environments table: Update/Updates/
// UpdateColumn(s)/Save/Create on a chain or query variable rooted in Model(<Environment>)
// or Table("environments"), Save/Updates/UpdateColumns/Create of an Environment value, or
// Exec/Raw of constant SQL that updates, inserts into or replaces into environments.
func (s *hostWriterScan) writes(recv *ast.FieldList, params *ast.FuncType, body ast.Node) bool {
	s.envs, s.queries, s.funcs, s.rawSQL = map[string]bool{}, map[string]bool{}, map[string]string{}, false
	ast.Inspect(body, func(n ast.Node) bool {
		if e, ok := n.(ast.Expr); ok {
			if text, ok := s.text(e); ok {
				s.rawSQL = s.rawSQL || hostRawWrites(text)
				return false
			}
		}
		return true
	})
	if params != nil {
		for _, list := range []*ast.FieldList{recv, params.Params, params.Results} {
			for _, field := range fieldList(list) {
				for _, n := range field.Names {
					s.envs[n.Name] = s.envs[n.Name] || s.isModelType(field.Type)
				}
			}
		}
	}
	for changed := true; changed; {
		changed = false
		ast.Inspect(body, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.ValueSpec:
				for i, name := range x.Names {
					if x.Type != nil && s.isModelType(x.Type) && !s.envs[name.Name] {
						s.envs[name.Name], changed = true, true
					}
					if i < len(x.Values) && s.bind(name, x.Values[i]) {
						changed = true
					}
				}
			case *ast.AssignStmt:
				for i, lhs := range x.Lhs {
					if len(x.Rhs) == len(x.Lhs) && s.bind(lhs, x.Rhs[i]) || len(x.Rhs) == 1 && i == 0 && s.bind(lhs, x.Rhs[0]) {
						changed = true
					}
				}
			case *ast.RangeStmt:
				if x.Value != nil && s.isEnv(x.X) {
					if id, ok := x.Value.(*ast.Ident); ok && !s.envs[id.Name] {
						s.envs[id.Name], changed = true, true
					}
				}
			}
			return true
		})
	}
	writes := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || writes {
			return !writes
		}
		var receiver ast.Expr
		method := ""
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			receiver, method = sel.X, sel.Sel.Name
		}
		switch fn := s.funcs[hostHolder(call.Fun)]; fn {
		case "":
		case "scoped":
			writes = true
			return false
		default:
			method = fn
		}
		switch {
		case method == "Exec" || method == "Raw":
			// Any constant host table write statement in the body counts, also when the
			// SQL reaches Exec through a variable or a formatting call.
			writes = s.rawSQL
		case hostWriteMethods[method]:
			// Save, Updates, UpdateColumns and Create take an Environment value as their model.
			valueModel := method != "Update" && method != "UpdateColumn" && len(call.Args) > 0 && s.isEnv(call.Args[0])
			writes = valueModel || receiver != nil && s.scoped(receiver)
		}
		return !writes
	})
	return writes
}

func fieldList(list *ast.FieldList) []*ast.Field {
	if list == nil {
		return nil
	}
	return list.List
}

// statusRefs counts status references in a writer body: constant string expressions
// (each folded as one) naming status, and Status fields or keys.
func (s *hostWriterScan) statusRefs(body ast.Node) int {
	refs := 0
	ast.Inspect(body, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok && id.Name == "Status" {
			refs++
			return true
		}
		expr, ok := n.(ast.Expr)
		if !ok {
			return true
		}
		if text, ok := s.text(expr); ok {
			if hostStatusWord.MatchString(text) {
				refs++
			}
			return false
		}
		return true
	})
	return refs
}

// hostTableWriterFuncs maps "file:Receiver.Func" (or "file:var" for package-level function
// literals) to its status reference count for each writer of the environments table.
func hostTableWriterFuncs(file string, src *ast.File, info *types.Info, models map[string]bool) map[string]int {
	scan := &hostWriterScan{info: info, models: models}
	found := map[string]int{}
	for _, decl := range src.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Body == nil {
				continue
			}
			name := d.Name.Name
			if d.Recv != nil && len(d.Recv.List) == 1 {
				recv := d.Recv.List[0].Type
				if star, ok := recv.(*ast.StarExpr); ok {
					recv = star.X
				}
				if id, ok := recv.(*ast.Ident); ok {
					name = id.Name + "." + name
				}
			}
			if scan.writes(d.Recv, d.Type, d.Body) {
				found[file+":"+name] = scan.statusRefs(d.Body)
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || len(vs.Names) == 0 {
					continue
				}
				for _, value := range vs.Values {
					ast.Inspect(value, func(n ast.Node) bool {
						lit, ok := n.(*ast.FuncLit)
						if ok && scan.writes(nil, lit.Type, lit.Body) {
							found[file+":"+vs.Names[0].Name] = scan.statusRefs(lit.Body)
						}
						return !ok
					})
				}
			}
		}
	}
	return found
}

func TestHostTableWritersAreTheReviewedSet(t *testing.T) {
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	fset := token.NewFileSet()
	packages := map[string]map[string]*ast.File{}
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
		dir := filepath.Dir(rel)
		if packages[dir] == nil {
			packages[dir] = map[string]*ast.File{}
		}
		packages[dir][filepath.ToSlash(rel)] = src
		return nil
	}))
	found := map[string]int{}
	for _, files := range packages {
		list := make([]*ast.File, 0, len(files))
		for _, src := range files {
			list = append(list, src)
		}
		info := hostConstantInfo(fset, list)
		models := hostModels(list, info)
		for rel, src := range files {
			for name, refs := range hostTableWriterFuncs(rel, src, info, models) {
				found[name] = refs
			}
		}
	}
	require.Equal(t, hostTableWriters, found, "a new host table writer or a changed status reference count needs classification in the reviewed writer table")
}

func scanHostWriterFixture(t *testing.T, src string) map[string]int {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "other.go", src, parser.SkipObjectResolution)
	require.NoError(t, err)
	info := hostConstantInfo(fset, []*ast.File{file})
	return hostTableWriterFuncs("internal/other/other.go", file, info, hostModels([]*ast.File{file}, info))
}

// The guard recognises the ordinary forms of a host table write, in any package.
func TestHostTableWriterGuardRecognisesWriteForms(t *testing.T) {
	forms := map[string]string{
		"map key":          `db.Model(&Environment{}).Where("id = ?", id).Updates(map[string]any{"Status": "pending"})`,
		"struct updates":   `db.Model(&environment.Environment{}).Where("id = ?", id).Updates(Environment{Status: "pending"})`,
		"struct save":      `var env Environment; env.Status = "pending"; db.Save(&env)`,
		"update column":    `db.Model(&Environment{}).Where("id = ?", id).UpdateColumn("status", "pending")`,
		"raw sql":          "db.Exec(`UPDATE environments SET status = 'pending' WHERE id = ?`, id)",
		"constant key":     `db.Table("environments").Where("id = ?", id).Updates(map[string]any{column: "pending"})`,
		"loaded row":       `env, _ := s.GetEnvironmentByID(ctx, id); db.Model(env).Update("status", "pending")`,
		"value model":      `env := &Environment{ID: id, Status: "pending"}; db.Updates(env)`,
		"query variable":   `q := db.Model(&Environment{}); q = q.Where("id = ?", id); q.Update("status", "pending")`,
		"var query":        `var q = db.Table("environments"); q.UpdateColumn("status", "pending")`,
		"var value":        `var row = environment.Environment{Status: "pending"}; row.ID = id; db.Save(&row)`,
		"new value":        `row := new(environment.Environment); row.ID = id; row.Status = "pending"; db.Save(row)`,
		"constant sql":     `db.Exec(resetSQL, id)`,
		"sql concat":       `db.Exec("UPDATE " + "environments SET status = ?", "pending")`,
		"range row":        `var rows []Environment; db.Find(&rows); for _, row := range rows { db.Model(&row).Update("status", "pending") }`,
		"upsert":           `db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&Environment{ID: id, Status: "pending"})`,
		"insert or update": "db.Exec(`INSERT OR REPLACE INTO environments (id, status) VALUES (?, ?)`, id, \"pending\")",
		"update or ignore": "db.Exec(`UPDATE OR IGNORE environments SET status = ? WHERE id = ?`, \"pending\", id)",
		"bracket quote":    "db.Exec(`UPDATE [environments] SET status = ? WHERE id = ?`, \"pending\", id)",
		"sql variable":     "sql := `UPDATE environments SET status = ? WHERE id = ?`; db.Exec(sql, \"pending\", id)",
		"formatted sql":    "db.Exec(fmt.Sprintf(\"UPDATE environments SET %s = ? WHERE id = ?\", \"status\"), \"pending\", id)",
		"schema table":     `db.Table("main.environments").Where("id = ?", id).Update("status", "pending")`,
		"quoted table":     "db.Table(`\"environments\" AS e`).Where(\"id = ?\", id).Update(\"status\", \"pending\")",
		"block comment":    "db.Exec(`UPDATE /* verdict */ environments SET status = ? WHERE id = ?`, \"pending\", id)",
		"line comment":     "db.Exec(\"UPDATE -- verdict\\n environments SET status = ? WHERE id = ?\", \"pending\", id)",
		"schema comment":   "db.Exec(`UPDATE main /* schema */ . environments SET status = ? WHERE id = ?`, \"pending\", id)",
		"upper table":      `db.Table("ENVIRONMENTS").Where("id = ?", id).Update("status", "pending")`,
		"mixed table":      `db.Table("Environments").Where("id = ?", id).Update("status", "pending")`,
		"spaced schema":    `db.Table("main . environments").Where("id = ?", id).Update("status", "pending")`,
		"query field":      `var h struct{ q *gorm.DB }; h.q = db.Table("environments").Where("id = ?", id); h.q.Update("status", "pending")`,
		"query struct lit": `h := holder{q: db.Table("environments")}; h.q.Update("status", "pending")`,
		"query map":        `qs := map[string]*gorm.DB{"host": db.Table("environments").Where("id = ?", id)}; qs["host"].Update("status", "pending")`,
		"method value":     `write := db.Table("environments").Where("id = ?", id).Update; write("status", "pending")`,
		"save value":       `row := new(Environment); row.Status = "pending"; save := db.Save; save(row)`,
		"exec value":       "exec := db.Exec; exec(`UPDATE environments SET status = ? WHERE id = ?`, \"pending\", id)",
		"local alias":      `type Host = Environment; var row Host; row.Status = "pending"; db.Save(&row)`,
		"pointer alias":    `type Host = *Environment; var row Host; row.Status = "pending"; db.Save(row)`,
		"slice alias":      `type Hosts = []Environment; var rows Hosts; for i := range rows { rows[i].Status = "pending" }; db.Save(&rows)`,
		"schema prefix":    "db.Exec(`UPDATE \"main\".\"environments\" SET status = ? WHERE id = ?`, \"pending\", id)",
		"concat const sql": `db.Exec(concatSQL, "pending", id)`,
		"alias const sql":  `db.Exec(aliasSQL, "pending", id)`,
		"paren query":      `q := db.Table("environments"); alias := (q); alias.UpdateColumn("status", "pending")`,
		"paren model":      `row := (new(environment.Environment)); row.ID = id; row.Status = "pending"; db.Save(row)`,
		"deref model":      `row := new(environment.Environment); row.ID = id; row.Status = "pending"; db.Save(*row)`,
		"concat const key": `db.Table("environments").UpdateColumn(splitColumn, "pending")`,
		"concat key":       `db.Table("environments").UpdateColumn("sta" + "tus", "pending")`,
	}
	for name, body := range forms {
		src := "package other\nconst column = \"status\"\nconst splitColumn = \"sta\" + \"tus\"\n" +
			"const concatSQL = \"UPDATE environments \" + \"SET status = ? WHERE id = ?\"\nconst aliasSQL = concatSQL\n" +
			"const resetSQL = `UPDATE environments SET status = 'pending' WHERE id = ?`\nfunc (s *Service) write() {\n" + body + "\n}\n"
		found := scanHostWriterFixture(t, src)
		require.Contains(t, found, "internal/other/other.go:Service.write", name)
		require.Positive(t, found["internal/other/other.go:Service.write"], name)
	}
	typed := scanHostWriterFixture(t, "package other\ntype Environment struct{ Status string }\ntype holder struct{ env *Environment }\n"+
		"func (h holder) load() *Environment { return h.env }\n"+
		"func (h holder) write(db *gorm.DB) { db.Save(h.env); db.Model(h.load()).Update(\"status\", \"pending\") }\n")
	require.Equal(t, map[string]int{"internal/other/other.go:holder.write": 1}, typed, "field and call results typed Environment")
	alias := scanHostWriterFixture(t, "package other\ntype hostRow struct{ Status string }\nfunc (hostRow) TableName() string { return \"environments\" }\n"+
		"func (s *Service) write(db *gorm.DB) { db.Model(&hostRow{}).Where(\"id = ?\", id).Update(\"status\", \"pending\") }\n")
	require.Equal(t, map[string]int{"internal/other/other.go:Service.write": 1}, alias, "TableName alias model")
	receiver := scanHostWriterFixture(t, "package other\nfunc (row *Environment) reset(db *gorm.DB) { row.Status = \"pending\"; db.Save(row) }\n")
	require.Equal(t, map[string]int{"internal/other/other.go:Environment.reset": 1}, receiver)
	literal := scanHostWriterFixture(t, "package other\nvar reset = func(db *gorm.DB) { db.Model(&Environment{}).Update(\"status\", \"pending\") }\n")
	require.Equal(t, map[string]int{"internal/other/other.go:reset": 1}, literal)
	reads := scanHostWriterFixture(t, "package other\nfunc (s *Service) read() {\ndb.Model(&Environment{}).Where(\"status = ?\", id).First(&env)\ndb.Model(&Run{}).Updates(map[string]any{\"status\": \"done\"})\ndb.Exec(\"UPDATE runs SET status = 'done'\")\ndb.Exec(\"UPDATE OR IGNORE [environments_archive] SET status = 'done'\")\n"+
		"db.Exec(\"UPDATE [environments-archive] SET status = 'done'\")\ndb.Exec(\"UPDATE environments.other SET status = 'done'\")\n"+
		"db.Table(\"environments_archive\").Update(\"status\", \"done\")\nwrite := db.Table(\"runs\").Update; write(\"status\", \"done\")\n}\n")
	require.Empty(t, reads)
}

// Constants resolve as the compiler binds them: lexical scope, implicitly repeated
// expressions, iota, typed constants and conversions; another function's constant of the
// same name never decides.
func TestHostTableWriterGuardResolvesConstantScopes(t *testing.T) {
	// Expected counts include a status literal of a local declaration and each use of it.
	cases := map[string]struct {
		src  string
		refs int
	}{
		"implicit sql": {
			"const (\nbaseSQL = \"UPDATE environments \" + \"SET status = ? WHERE id = ?\"\nresetSQL\n)\n" +
				"func (s *Service) write() { db.Exec(resetSQL, \"pending\", id) }\n", 1},
		"implicit key": {
			"const (\nbaseColumn = \"sta\" + \"tus\"\ncolumn\n)\n" +
				"func (s *Service) write() { db.Table(\"environments\").UpdateColumn(column, \"pending\") }\n", 1},
		"local sql": {
			"func (s *Service) write() { const query = \"UPDATE environments \" + \"SET status = ? WHERE id = ?\"; db.Exec(query, \"pending\", id) }\n" +
				"func (s *Service) read() { const query = \"SELECT 1\"; db.Raw(query) }\n", 2},
		"shadowed key": {
			"const column = \"sta\" + \"tus\"\nfunc (s *Service) other() { const column = \"color\"; _ = column }\n" +
				"func (s *Service) write() { db.Model(&Environment{}).Where(\"id = ?\", id).Update(column, \"pending\") }\n", 1},
		"typed conversion": {
			"type column string\nconst (\nfirst column = \"sta\" + \"tus\"\nsecond\n)\nconst third = string(second)\n" +
				"func (s *Service) write() { db.Table(\"environments\").UpdateColumn(third, \"pending\") }\n", 1},
		"inner block": {
			"func (s *Service) write() { const column = \"color\"; { const column = \"status\"; db.Table(\"environments\").UpdateColumn(column, \"pending\") } }\n", 2},
	}
	for name, c := range cases {
		found := scanHostWriterFixture(t, "package other\n"+c.src)
		require.Equal(t, map[string]int{"internal/other/other.go:Service.write": c.refs}, found, name)
	}
	outer := scanHostWriterFixture(t, "package other\nfunc (s *Service) write() { const column = \"color\"; { const column = \"status\" }; db.Table(\"environments\").UpdateColumn(column, \"x\") }\n")
	require.Equal(t, map[string]int{"internal/other/other.go:Service.write": 1}, outer, "the outer use resolves to the outer constant")
}

// A status reference added inside an already reviewed writer changes its pinned count.
func TestHostTableWriterGuardCountsStatusReferences(t *testing.T) {
	base := "package other\nfunc (s *Service) edit(updates map[string]any) {\n%s\ndb.Model(&Environment{}).Where(\"id = ?\", id).Updates(updates)\n}\n"
	clean := scanHostWriterFixture(t, strings.Replace(base, "%s", "", 1))
	reset := scanHostWriterFixture(t, strings.Replace(base, "%s", `if _, ok := updates["color"]; ok { updates["status"] = "pending" }`, 1))
	field := scanHostWriterFixture(t, strings.Replace(base, "%s", `var env Environment; env.Status = "pending"; updates["x"] = env`, 1))
	require.Equal(t, 0, clean["internal/other/other.go:Service.edit"])
	require.Equal(t, 1, reset["internal/other/other.go:Service.edit"])
	require.Equal(t, 1, field["internal/other/other.go:Service.edit"])
	folded := scanHostWriterFixture(t, strings.Replace(base, "%s", `db.Model(&Environment{}).Where("id = ?", id).Update("sta" + "tus", "pending")`, 1))
	require.Equal(t, 1, folded["internal/other/other.go:Service.edit"])
}

// The generic edit sink accepts only its editable columns, so no spelling of the status
// column (GORM field name, SQLite case variants) can replace a verdict; refused edits
// change nothing and successful edits persist and keep it.
func TestGenericHostUpdateRefusesStatus(t *testing.T) {
	f := newAgentFixture(t, "edge", version.Info{})
	require.NoError(t, f.service.db.Model(&Environment{}).Where("id = ?", f.id).Update("status", "update-required").Error)
	before, err := f.service.GetEnvironmentByID(t.Context(), f.id)
	require.NoError(t, err)
	stored := func(label string) *Environment {
		t.Helper()
		row, err := f.service.GetEnvironmentByID(t.Context(), f.id)
		require.NoError(t, err)
		require.Equal(t, "update-required", row.Status, label)
		return row
	}
	for _, key := range []string{"status", "Status", "STATUS", "sTaTuS", "last_seen", "is_edge"} {
		_, err := f.service.UpdateEnvironment(t.Context(), f.id, map[string]any{key: "pending", "name": "Renamed"}, nil, nil)
		require.Error(t, err, key)
		require.Equal(t, before.Name, stored(key).Name, key)
	}
	edits := []struct {
		label string
		edit  map[string]any
		check func(*Environment) bool
	}{
		{"name", map[string]any{"name": "Example renamed"}, func(e *Environment) bool { return e.Name == "Example renamed" }},
		{"colour set", map[string]any{"color": "#336699"}, func(e *Environment) bool { return e.Color != nil && *e.Color == "#336699" }},
		{"colour null", map[string]any{"color": nil}, func(e *Environment) bool { return e.Color == nil }},
		{"disable", map[string]any{"enabled": false}, func(e *Environment) bool { return !e.Enabled }},
		{"enable", map[string]any{"enabled": true}, func(e *Environment) bool { return e.Enabled }},
	}
	for _, step := range edits {
		_, err := f.service.UpdateEnvironment(t.Context(), f.id, step.edit, nil, nil)
		require.NoError(t, err, step.label)
		require.True(t, step.check(stored(step.label)), step.label)
	}
}
