package datasource

import (
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// TestAcceptedTypesComeFromTheRegistry closes the gap between two lists that
// had no reason to agree.
//
// ValidDatasourceTypes was a hand-written map, consulted at five entry points,
// while SQL templates and connection testing asked driver.IsRegistered. A
// driver registered but missing from the map produced a datasource nobody could
// create and a template anybody could — and nothing failed to tell you.
func TestAcceptedTypesComeFromTheRegistry(t *testing.T) {
	for _, typ := range driver.SupportedTypes() {
		if !IsValidDatasourceType(typ) {
			t.Errorf("driver %q is registered but rejected at the datasource entry points", typ)
		}
	}

	for _, typ := range []string{"", "oracle", "postgres", "MySQL"} {
		if IsValidDatasourceType(typ) {
			t.Errorf("%q was accepted, but no driver is registered for it", typ)
		}
	}
}

// TestTypeErrorMessageListsTheRegisteredTypes keeps the message truthful.
//
// It was two hardcoded copies of the same sentence, in datasource.go and
// handler.go. Adding a driver meant remembering both, and forgetting left the
// user a list that omitted a type the server would actually accept.
func TestTypeErrorMessageListsTheRegisteredTypes(t *testing.T) {
	msg := ErrInvalidDatasourceType.Error()
	for _, typ := range driver.SupportedTypes() {
		if !strings.Contains(msg, typ) {
			t.Errorf("the rejection message omits the supported type %q: %s", typ, msg)
		}
	}
}
