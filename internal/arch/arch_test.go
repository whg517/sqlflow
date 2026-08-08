// Package arch holds no code. Its test is the executable statement of the
// package layering, so that a violation fails CI instead of being noticed in
// review — or not at all.
package arch_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/whg517/sqlflow/"

// domains are the vertical slices. Each owns its service, its HTTP handlers and
// its tests, and each is free to depend on platform and on the shared kernel.
var domains = map[string]bool{
	"audit": true, "datasource": true, "iam": true, "notify": true,
	"ops": true, "query": true, "security": true, "ticket": true,
}

// allowedDomainEdges lists every dependency one domain's production code may
// have on another's.
//
// The set is deliberately small and explicit: a new entry is a decision to
// couple two slices, which should be argued for in review rather than
// discovered later. Prefer an interface declared at the point of use — see
// datasource.ObjectViewChecker — over adding an edge here.
var allowedDomainEdges = map[string]map[string]string{
	"query": {
		"datasource": "查询要解析数据源配置并取连接",
		"security":   "查询前检查表级权限并应用脱敏规则",
	},
	"ticket": {
		"datasource": "工单执行变更需要目标数据源",
		"notify":     "工单状态流转发送通知",
		"ops":        "工单关联 Git 提交",
		"security":   "MongoDB 工单做集合级权限检查",
	},
}

// allowedTestOnlyEdges are dependencies that exist only in _test.go files.
//
// They are held to a looser standard than production edges because they do not
// constrain the deployed dependency graph, but they are still declared: a test
// reaching into another domain is where boundary erosion usually starts, and an
// edge that silently appears in both lists means production coupling slipped in
// behind a test.
var allowedTestOnlyEdges = map[string]map[string]string{
	"query":    {"audit": "断言审计行确实落库，需要真实的 audit.Service 而非 auditlog.Writer"},
	"ticket":   {"audit": "同上：工单流转的审计留痕断言"},
	"security": {"audit": "同上：脱敏规则变更的审计留痕断言"},
}

// TestPlatformDoesNotDependOnDomains guards the one rule that keeps platform
// reusable: it is infrastructure, so it cannot know about the business.
//
// A violation is usually a helper that drifted — it started generic, then grew
// a domain type in its signature. The fix is to move the domain-specific part
// back out, not to add an exception.
func TestPlatformDoesNotDependOnDomains(t *testing.T) {
	for pkg, deps := range packageImports(t) {
		if !strings.HasPrefix(pkg, "internal/platform/") {
			continue
		}
		for _, imp := range deps.all() {
			if d := domainOf(imp); domains[d] {
				t.Errorf("%s imports %s: platform 不能依赖领域包", pkg, imp)
			}
		}
	}
}

// TestDomainsDoNotDependOnTransport guards the direction between a domain and
// the HTTP layer.
//
// Domains serve HTTP — each owns its handlers — but they must not reach back
// into internal/api. Routing, middleware and cross-domain endpoints live there,
// and a domain that imported them would make the composition root's job
// ambiguous and reintroduce the import cycle the split removed.
func TestDomainsDoNotDependOnTransport(t *testing.T) {
	for pkg, deps := range packageImports(t) {
		if !domains[domainOf(pkg)] {
			continue
		}
		for _, imp := range deps.all() {
			if strings.HasPrefix(imp, "internal/api") {
				t.Errorf("%s imports %s: 领域包不能依赖传输层（应在消费侧声明接口）", pkg, imp)
			}
		}
	}
}

// TestDomainEdgesAreDeclared fails on any cross-domain dependency that is not
// declared, and on any declared edge that no longer exists.
//
// The second half matters as much as the first: a stale entry reads as
// permission for coupling that was already removed, and would let it come back
// unnoticed.
func TestDomainEdgesAreDeclared(t *testing.T) {
	prod, test := crossDomainEdges(t)

	for from, tos := range prod {
		for to := range tos {
			if _, ok := allowedDomainEdges[from][to]; !ok {
				t.Errorf("未声明的跨领域依赖: %s -> %s\n"+
					"  若确实需要，请在 allowedDomainEdges 中说明理由；\n"+
					"  若只用到对方一两个方法，优先在消费侧声明接口。", from, to)
			}
		}
	}
	for from, tos := range test {
		for to := range tos {
			if _, ok := allowedDomainEdges[from][to]; ok {
				continue // production already allows it, so tests may too
			}
			if _, ok := allowedTestOnlyEdges[from][to]; !ok {
				t.Errorf("未声明的跨领域测试依赖: %s -> %s\n"+
					"  请在 allowedTestOnlyEdges 中说明为何测试需要对方的具体实现。", from, to)
			}
		}
	}

	for from, tos := range allowedDomainEdges {
		for to, why := range tos {
			if !prod[from][to] {
				t.Errorf("allowedDomainEdges 中的 %s -> %s（%q）已不存在，请删除该条目", from, to, why)
			}
		}
	}
	for from, tos := range allowedTestOnlyEdges {
		for to, why := range tos {
			if prod[from][to] {
				t.Errorf("%s -> %s 被登记为「仅测试」，但生产代码也依赖了它（%q）。\n"+
					"  这正是 auditlog.Writer 这类窄接口要防止的耦合。", from, to, why)
			}
			if !test[from][to] {
				t.Errorf("allowedTestOnlyEdges 中的 %s -> %s（%q）已不存在，请删除该条目", from, to, why)
			}
		}
	}
}

// crossDomainEdges returns the cross-domain dependencies of production code and
// of test code separately.
func crossDomainEdges(t *testing.T) (prod, test map[string]map[string]bool) {
	t.Helper()
	prod, test = map[string]map[string]bool{}, map[string]map[string]bool{}
	add := func(m map[string]map[string]bool, from, to string) {
		if m[from] == nil {
			m[from] = map[string]bool{}
		}
		m[from][to] = true
	}
	for pkg, deps := range packageImports(t) {
		from := domainOf(pkg)
		if !domains[from] {
			continue
		}
		for _, imp := range deps.production {
			if to := domainOf(imp); domains[to] && to != from {
				add(prod, from, to)
			}
		}
		for _, imp := range deps.test {
			if to := domainOf(imp); domains[to] && to != from {
				add(test, from, to)
			}
		}
	}
	return prod, test
}

// TestAppIsTheOnlyCompositionRoot verifies that nothing outside internal/app
// and cmd/ imports more than a couple of domains.
//
// Knowing every concrete implementation is the composition root's job. When
// another package accumulates that knowledge it has quietly become a second
// wiring point, and the two drift.
func TestAppIsTheOnlyCompositionRoot(t *testing.T) {
	const maxDomainsElsewhere = 2
	for pkg, deps := range packageImports(t) {
		switch {
		case pkg == "internal/app", strings.HasPrefix(pkg, "cmd/"):
			continue
		// The router wires every domain's handlers onto routes by definition.
		case pkg == "internal/api":
			continue
		case domains[domainOf(pkg)]:
			continue
		}
		seen := map[string]bool{}
		for _, imp := range deps.all() {
			if d := domainOf(imp); domains[d] {
				seen[d] = true
			}
		}
		if len(seen) > maxDomainsElsewhere {
			t.Errorf("%s 依赖了 %d 个领域包 %v：组合根应当只有 internal/app",
				pkg, len(seen), sortedKeys(seen))
		}
	}
}

// TestDomainsDoNotQueryThroughDatabaseSQL is ADR-0010's exit condition, made
// executable.
//
// Ent is the only way domain code reaches the platform store. The rule is not
// about taste: the two-track period cost a batch of defects that a typed query
// layer makes impossible to write — placeholders numbered by hand, ON CONFLICT
// and INSERT OR IGNORE spelled for the wrong dialect, booleans written as 0/1,
// ids read with LastInsertId. Each of those was a valid Go program that failed
// at runtime, and several failed silently.
//
// The check looks for calls to database/sql's query methods on anything other
// than a transaction handle, since ent's own Tx exposes the same names. It is
// deliberately syntactic — a domain that wants raw SQL badly enough can defeat
// it — so its job is to make the decision visible, not to be airtight.
func TestDomainsDoNotQueryThroughDatabaseSQL(t *testing.T) {
	// Method names database/sql exposes for running statements. ent's builders
	// use Exec, Query and QueryRow too, but only ever on a client or a builder
	// value — never on a *sql.DB the domain is holding.
	sqlMethods := map[string]bool{
		"QueryContext": true, "QueryRowContext": true, "ExecContext": true,
	}

	root := repoRoot(t)
	fset := token.NewFileSet()
	var offenders []string

	for domain := range domains {
		dir := filepath.Join(root, "internal", domain)
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			file, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				t.Fatalf("parse %s: %v", path, err)
			}
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !sqlMethods[sel.Sel.Name] {
					return true
				}
				offenders = append(offenders, fmt.Sprintf("internal/%s/%s:%d: %s",
					domain, e.Name(), fset.Position(call.Pos()).Line, sel.Sel.Name))
				return true
			})
		}
	}

	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Errorf("领域包不得直接用 database/sql 查询平台库（见 ADR-0010），发现 %d 处：\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// domainOf returns the second path element of an internal package, which is the
// domain name: "internal/query/handler_export.go" -> "query".
func domainOf(pkg string) string {
	parts := strings.Split(pkg, "/")
	if len(parts) < 2 || parts[0] != "internal" {
		return ""
	}
	return parts[1]
}

// imports are a package's first-party dependencies, split by whether they come
// from production or test files.
type imports struct {
	production []string
	test       []string
}

// all returns every first-party import of the package, from both kinds of file.
func (i imports) all() []string {
	return append(append([]string{}, i.production...), i.test...)
}

// packageImports maps each first-party package to the first-party packages it
// imports.
func packageImports(t *testing.T) map[string]imports {
	t.Helper()
	root := repoRoot(t)
	prod := map[string]map[string]bool{}
	test := map[string]map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "web", "ent":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		if !strings.HasPrefix(pkg, "internal/") && !strings.HasPrefix(pkg, "cmd/") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		bucket := prod
		if strings.HasSuffix(path, "_test.go") {
			bucket = test
		}
		if bucket[pkg] == nil {
			bucket[pkg] = map[string]bool{}
		}
		if prod[pkg] == nil {
			prod[pkg] = map[string]bool{}
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if rest, ok := strings.CutPrefix(imported, module); ok {
				bucket[pkg][rest] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
	if len(prod) == 0 {
		t.Fatal("no packages found — the walk root is probably wrong")
	}

	out := make(map[string]imports, len(prod))
	for pkg, deps := range prod {
		out[pkg] = imports{production: sortedKeys(deps), test: sortedKeys(test[pkg])}
	}
	return out
}

// repoRoot walks up from the test's working directory to the directory holding
// go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above the test directory")
		}
		dir = parent
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// datasourceTypeNames are the identifiers the driver registry keys on.
//
// They are business vocabulary: which data sources this platform governs is a
// product decision, and it changes. A platform package that branches on one has
// taken a position on it.
var datasourceTypeNames = map[string]bool{
	"mysql": true, "postgresql": true, "postgres": true, "pg": true,
	"mongodb": true, "mongo": true, "elasticsearch": true, "es": true,
	"sqlite": true, "sqlite3": true,
}

// TestPlatformDoesNotBranchOnDatasourceType closes the gap the import check
// leaves open.
//
// TestPlatformDoesNotDependOnDomains catches a platform package that imports a
// domain. It cannot catch one that hardcodes the domain's vocabulary instead,
// and that is what sqlparser did: a `switch dbType` over every driver name,
// which made it a third registry alongside internal/driver and the datasource
// type whitelist. Adding a driver meant editing a package that is supposed to
// know nothing about drivers, and forgetting to produced "unsupported database
// type" at runtime rather than a compile error.
//
// Only comparisons count. A parser dedicated to MongoDB may say "MongoDB" in
// its error messages — that is the product's name, not a branch on it.
func TestPlatformDoesNotBranchOnDatasourceType(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	var offenders []string

	report := func(rel string, pos token.Pos, value, form string) {
		offenders = append(offenders, fmt.Sprintf("%s:%d: %s %q",
			rel, fset.Position(pos).Line, form, value))
	}

	// matched reports the type name a literal holds, if any.
	matched := func(e ast.Expr) (string, bool) {
		lit, ok := e.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return "", false
		}
		v, err := strconv.Unquote(lit.Value)
		if err != nil {
			return "", false
		}
		return v, datasourceTypeNames[strings.ToLower(v)]
	}

	err := filepath.WalkDir(filepath.Join(root, "internal", "platform"),
		func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, parseErr := parser.ParseFile(fset, path, nil, 0)
			if parseErr != nil {
				return parseErr
			}
			rel, _ := filepath.Rel(root, path)
			rel = filepath.ToSlash(rel)

			ast.Inspect(file, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.CaseClause:
					for _, e := range node.List {
						if v, ok := matched(e); ok {
							report(rel, e.Pos(), v, "switch case on")
						}
					}
				case *ast.BinaryExpr:
					if node.Op != token.EQL && node.Op != token.NEQ {
						return true
					}
					for _, e := range []ast.Expr{node.X, node.Y} {
						if v, ok := matched(e); ok {
							report(rel, e.Pos(), v, "compares against")
						}
					}
				}
				return true
			})
			return nil
		})
	if err != nil {
		t.Fatalf("walk platform: %v", err)
	}

	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Errorf("internal/platform branches on %d datasource type name(s); the driver owns that vocabulary:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestNoWriteOnlyFields fails on a field that is assigned and never read.
//
// staticcheck's `unused` does not report this: a field set by a constructor
// counts as used, so the value can be written on every path and read on none.
// Four such fields survived a full pass of the linter — PoolManager.lastUse,
// which was also the subject of a data race, poolEntry.config, ESIndexInfo's
// StoreBytes, and ticket Service.permSvc, left behind when the check that
// needed it came out.
//
// The scope is deliberately narrow, and the narrowness is what makes the result
// trustworthy rather than a source of //nolint noise:
//
//   - Unexported only. Every use is then inside the package, so a package-wide
//     scan is complete. An exported field can be read by any importer, and
//     answering that needs the whole-program call graph that make deadcode uses.
//   - Untagged only. A struct tag means something reads the field by
//     reflection — encoding/json, ent, a SQL scanner — and no syntactic scan
//     can see that call. StoreBytes had a json tag and is the miss this rule
//     accepts in exchange for having no false positives.
//
// Fields are keyed by name within a package rather than by (type, field),
// because a syntactic scan cannot always resolve what x.foo selects. Two
// structs sharing a field name are therefore treated as one, which under-reports
// — the right direction for a gate.
func TestNoWriteOnlyFields(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()

	type parsedFile struct {
		pkg  string
		file *ast.File
	}
	var parsed []parsedFile

	type field struct {
		pkg, structName, name, file string
		line                        int
	}
	var declared []field

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "node_modules", "web", "ent", "openapi", "testdata":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if !strings.HasPrefix(rel, "internal/") && !strings.HasPrefix(rel, "cmd/") {
			return nil
		}
		f, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		pkg := filepath.ToSlash(filepath.Dir(rel))
		parsed = append(parsed, parsedFile{pkg: pkg, file: f})

		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			st, ok := spec.Type.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, fld := range st.Fields.List {
				if fld.Tag != nil {
					continue
				}
				for _, name := range fld.Names {
					if ast.IsExported(name.Name) || name.Name == "_" {
						continue
					}
					declared = append(declared, field{
						pkg: pkg, structName: spec.Name.Name, name: name.Name,
						file: rel, line: fset.Position(name.Pos()).Line,
					})
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk repo: %v", err)
	}
	if len(declared) == 0 {
		t.Fatal("no unexported struct fields found — the walk root is probably wrong")
	}

	read := map[string]bool{}
	for _, pf := range parsed {
		// notRead holds the nodes that are a write or a declaration rather than
		// a read: the left side of an assignment, a composite-literal key, and
		// the field's own name in its struct type. Leaving out that last one
		// made every field look read, and the check reported nothing at all —
		// which is why it is sabotage-tested rather than trusted for returning
		// zero.
		notRead := map[ast.Node]bool{}
		ast.Inspect(pf.file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				if node.Tok != token.ASSIGN && node.Tok != token.DEFINE {
					return true // x.f += v reads before it writes
				}
				for _, lhs := range node.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok {
						notRead[sel] = true
					}
				}
			case *ast.CompositeLit:
				for _, elt := range node.Elts {
					if kv, ok := elt.(*ast.KeyValueExpr); ok {
						if id, ok := kv.Key.(*ast.Ident); ok {
							notRead[id] = true
						}
					}
				}
			case *ast.StructType:
				if node.Fields == nil {
					return true
				}
				for _, fld := range node.Fields.List {
					for _, name := range fld.Names {
						notRead[name] = true
					}
				}
			}
			return true
		})

		ast.Inspect(pf.file, func(n ast.Node) bool {
			if notRead[n] {
				return true
			}
			switch node := n.(type) {
			case *ast.SelectorExpr:
				read[pf.pkg+"."+node.Sel.Name] = true
			case *ast.Ident:
				read[pf.pkg+"."+node.Name] = true
			}
			return true
		})
	}

	var offenders []string
	for _, f := range declared {
		if !read[f.pkg+"."+f.name] {
			offenders = append(offenders, fmt.Sprintf("%s:%d: %s.%s", f.file, f.line, f.structName, f.name))
		}
	}
	sort.Strings(offenders)
	if len(offenders) > 0 {
		t.Errorf("%d field(s) are written and never read; delete them or read them:\n  %s",
			len(offenders), strings.Join(offenders, "\n  "))
	}
}

// TestTicketStatusHasOneWriter keeps the ticket state machine enforceable.
//
// internal/ticket declares a state machine and a compare-and-swap helper that
// consults it, but for a long time neither was binding: the table had no
// production caller at all, and five lifecycle writes reached for a bare
// UpdateOneID instead. That is how a cancel could report success while the
// statement it cancelled went on running, and how a decided ticket could be
// walked forward again — each site re-derived the guard, and five of them got
// it wrong.
//
// A deep module only helps if callers cannot step around it, and in Go nothing
// in the type system says "this column has an owner". This test says it: a
// ticket update that writes status may appear only in the file that owns the
// transition. Updates that touch other columns — the SLA deadline, say — are
// untouched, because they are not state changes.
func TestTicketStatusHasOneWriter(t *testing.T) {
	const owner = "transition.go"

	dir := filepath.Join(repoRoot(t), "internal", "ticket")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read internal/ticket: %v", err)
	}

	fset := token.NewFileSet()
	var offenders []string

	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || name == owner {
			continue
		}

		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != "SetStatus" {
				return true
			}
			// Walk back down the method chain. A ticket *update* that writes
			// status is the thing being banned; Ticket.Create() sets the status
			// a row is born with, and TicketRevision/ExecutionResult are other
			// tables entirely.
			chain := selectorChain(sel.X)
			if chain["Ticket"] && (chain["Update"] || chain["UpdateOneID"]) {
				offenders = append(offenders, fmt.Sprintf("%s:%d",
					name, fset.Position(call.Pos()).Line))
			}
			return true
		})
	}

	if len(offenders) > 0 {
		sort.Strings(offenders)
		t.Errorf("%d ticket status write(s) bypass %s:\n  %s\n"+
			"  状态迁移必须走 applyTransition：它校验状态机声明、以 CAS 落库，"+
			"并能在同一条谓词里守住审批阶段。",
			len(offenders), owner, strings.Join(offenders, "\n  "))
	}
}

// selectorChain collects every identifier and selector name in a method chain,
// so a caller can ask what a fluent builder was rooted at.
func selectorChain(e ast.Expr) map[string]bool {
	names := map[string]bool{}
	for {
		switch v := e.(type) {
		case *ast.CallExpr:
			e = v.Fun
		case *ast.SelectorExpr:
			names[v.Sel.Name] = true
			e = v.X
		case *ast.Ident:
			names[v.Name] = true
			return names
		default:
			return names
		}
	}
}
