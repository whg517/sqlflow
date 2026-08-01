// Package arch holds no code. Its test is the executable statement of the
// package layering, so that a violation fails CI instead of being noticed in
// review — or not at all.
package arch_test

import (
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
