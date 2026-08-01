// Package all registers every bundled data source driver as a side effect of
// being imported.
//
// Driver registration happens in each driver package's init(), so a binary that
// forgets one of the blank imports silently loses that data source type — the
// registry lookup fails at runtime rather than at compile time. Importing this
// package once keeps the entry point and the tests on the same driver set.
package all

import (
	_ "github.com/whg517/sqlflow/internal/driver/elasticsearch"
	_ "github.com/whg517/sqlflow/internal/driver/mongodb"
	_ "github.com/whg517/sqlflow/internal/driver/mysql"
	_ "github.com/whg517/sqlflow/internal/driver/postgresql"
	_ "github.com/whg517/sqlflow/internal/driver/sqlite"
)
