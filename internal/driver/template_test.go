package driver_test

import (
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// The template renderer used to switch on the data source type to decide the
// placeholder syntax. These tests pin the replacement: each driver states its
// own dialect, so a new engine needs no change in the renderer.

func TestTemplateDialectFor_PlaceholderSyntax(t *testing.T) {
	tests := []struct {
		typeName  string
		wantBinds bool
		// want is the placeholder at positions 1, 2, 3.
		want []string
	}{
		{"mysql", true, []string{"?", "?", "?"}},
		{"sqlite", true, []string{"?", "?", "?"}},
		{"postgresql", true, []string{"$1", "$2", "$3"}},
		{"mongodb", false, nil},
		{"elasticsearch", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.typeName, func(t *testing.T) {
			dialect, err := driver.TemplateDialectFor(tt.typeName)
			if err != nil {
				t.Fatalf("TemplateDialectFor: %v", err)
			}
			if dialect.Binds != tt.wantBinds {
				t.Fatalf("Binds = %v, want %v", dialect.Binds, tt.wantBinds)
			}
			if !tt.wantBinds {
				if dialect.Placeholder != nil {
					t.Error("Placeholder must be nil when the driver does not bind")
				}
				return
			}
			for i, want := range tt.want {
				if got := dialect.Placeholder(i + 1); got != want {
					t.Errorf("Placeholder(%d) = %q, want %q", i+1, got, want)
				}
			}
		})
	}
}

// A document source consumes a JSON body, so a rendered template travels in a
// different payload shape than a SQL statement.
func TestTemplateDialectFor_QueryForm(t *testing.T) {
	for typeName, want := range map[string]driver.QueryForm{
		"mysql":         driver.QueryFormSQL,
		"postgresql":    driver.QueryFormSQL,
		"mongodb":       driver.QueryFormDocument,
		"elasticsearch": driver.QueryFormDSL,
	} {
		dialect, err := driver.TemplateDialectFor(typeName)
		if err != nil {
			t.Fatalf("TemplateDialectFor(%q): %v", typeName, err)
		}
		if dialect.QueryForm != want {
			t.Errorf("%s: QueryForm = %q, want %q", typeName, dialect.QueryForm, want)
		}
	}
}

func TestTemplateDialectFor_UnknownType(t *testing.T) {
	if _, err := driver.TemplateDialectFor("oracle"); err == nil {
		t.Fatal("an unregistered type should be rejected")
	}
}
