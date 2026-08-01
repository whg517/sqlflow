package testutil

// Registering every driver here means a test that seeds a datasource works in
// any package, instead of depending on whichever test file in that package
// happens to carry the blank import. Extracting internal/service into domain
// packages moved one such file and silently emptied the registry for the
// package it left behind; the failure surfaced only as "supported: []".
import _ "github.com/whg517/sqlflow/internal/driver/all"
