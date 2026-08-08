package datasource

import (
	"errors"
	"fmt"
	"strings"
)

// ErrDatabaseScopeMismatch is returned when a caller asks to run against a
// database the datasource is not connected to.
var ErrDatabaseScopeMismatch = errors.New("请求的数据库与数据源配置不一致")

// ResolveQueryScope returns the database a query on this datasource will
// actually run in.
//
// The scope is a property of the datasource row, not of the request. The
// connection pool keys on datasource ID alone, the DSN pins the database at
// Connect, and nothing in the platform issues USE or SET search_path — so
// `configured` is the only answer that can be true. An empty request means
// "wherever this datasource points", which is the normal case.
//
// A request naming something else is refused rather than ignored, and that is
// the whole point of this function. The caller-supplied string used to be
// discarded by the driver while still narrowing the mask-rule lookup, which
// kept only the rules matching that database or no database at all. Naming a
// database no rule was scoped to therefore dropped every rule protecting the
// database the rows came from — the query still ran where it always ran, and
// returned it unmasked. Silently substituting the real scope would close that
// hole but leave the caller believing they read somewhere they did not, and the
// audit row would say so too; the honest answer to "run this against X" when
// only Y is reachable is no.
//
// Comparison is case-insensitive because the returned scope is `configured`
// either way: matching loosely only decides whether the caller gets an error,
// never which database is read.
func ResolveQueryScope(configured, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" || strings.EqualFold(requested, strings.TrimSpace(configured)) {
		return configured, nil
	}
	if configured == "" {
		return "", fmt.Errorf("%w: 数据源未配置数据库，无法执行对 %q 的查询", ErrDatabaseScopeMismatch, requested)
	}
	return "", fmt.Errorf("%w: 该数据源固定连接 %q，无法执行对 %q 的查询", ErrDatabaseScopeMismatch, configured, requested)
}
