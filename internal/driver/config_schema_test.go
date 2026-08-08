package driver_test

import (
	"slices"
	"testing"

	"github.com/whg517/sqlflow/internal/driver"
	_ "github.com/whg517/sqlflow/internal/driver/all"
)

// TestDatasourceTypes_CoversEveryRegisteredDriver pins the seam the datasource
// form renders from.
//
// The form used to hold the type list twice — a hardcoded <SelectItem> per type
// and a default-port map — so a registered driver the UI had not been told
// about could not be selected at all, and adding one meant editing a 1321-line
// component in about ten places.
func TestDatasourceTypes_CoversEveryRegisteredDriver(t *testing.T) {
	types, err := driver.DatasourceTypes()
	if err != nil {
		t.Fatalf("DatasourceTypes: %v", err)
	}

	got := map[string]driver.DatasourceTypeInfo{}
	for _, info := range types {
		got[info.Type] = info
	}
	for _, want := range productionDriverTypes {
		if _, ok := got[want]; !ok {
			t.Errorf("%s is registered but absent from DatasourceTypes", want)
		}
	}

	for _, info := range types {
		// Only the real drivers. Tests register their own doubles, and a mock's
		// empty form says nothing about whether a data source can be configured
		// — the same reason capability_meaning_test.go keeps its own list.
		if !slices.Contains(productionDriverTypes, info.Type) {
			continue
		}
		t.Run(info.Type, func(t *testing.T) {
			if len(info.Fields) == 0 {
				t.Fatal("declares no configuration fields, so its form would be empty")
			}
			if info.QueryForm == "" {
				t.Error("declares no query form")
			}

			seen := map[string]bool{}
			for _, f := range info.Fields {
				switch {
				case f.Name == "":
					t.Error("a field has no name, so its value has nowhere to go")
				case seen[f.Name]:
					t.Errorf("field %q is declared twice", f.Name)
				}
				seen[f.Name] = true

				switch f.Kind {
				case driver.FieldText, driver.FieldPassword, driver.FieldNumber,
					driver.FieldSelect, driver.FieldSwitch:
				default:
					t.Errorf("field %q has unknown kind %q", f.Name, f.Kind)
				}

				switch f.Storage {
				case driver.StorageColumn, driver.StorageExtra:
				default:
					t.Errorf("field %q has unknown storage %q — the form would not know where to send it", f.Name, f.Storage)
				}

				if f.Kind == driver.FieldSelect && len(f.Options) == 0 {
					t.Errorf("select field %q offers no options", f.Name)
				}
				// A condition naming a field that does not exist would hide the
				// dependent input forever.
				if f.ShowWhen != nil {
					found := false
					for _, other := range info.Fields {
						if other.Name == f.ShowWhen.Field {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("field %q is shown when %q has a value, but %q is not a field of this type",
							f.Name, f.ShowWhen.Field, f.ShowWhen.Field)
					}
				}
			}
		})
	}
}

// TestDatasourceTypes_SQLiteNeedsNoHost is the case the old form expressed by
// excluding SQLite from three separate blocks.
func TestDatasourceTypes_SQLiteNeedsNoHost(t *testing.T) {
	types, err := driver.DatasourceTypes()
	if err != nil {
		t.Fatalf("DatasourceTypes: %v", err)
	}
	for _, info := range types {
		if info.Type != "sqlite" {
			continue
		}
		for _, f := range info.Fields {
			if f.Name == "host" || f.Name == "port" || f.Name == "username" {
				t.Errorf("sqlite declares %q, which it has no use for", f.Name)
			}
		}
		if len(info.Fields) != 1 || info.Fields[0].Name != "database" {
			t.Errorf("sqlite fields = %+v, want just the file path", info.Fields)
		}
		return
	}
	t.Fatal("sqlite is not among the registered types")
}

// TestDatasourceTypes_ElasticsearchKeepsSettingsOutOfColumns pins the rule the
// backend already follows and the form did not.
func TestDatasourceTypes_ElasticsearchKeepsSettingsOutOfColumns(t *testing.T) {
	types, err := driver.DatasourceTypes()
	if err != nil {
		t.Fatalf("DatasourceTypes: %v", err)
	}
	for _, info := range types {
		if info.Type != "elasticsearch" {
			continue
		}
		for _, f := range info.Fields {
			switch f.Name {
			case "urls", "auth_type", "index_pattern", "verify_certs":
				if f.Storage != driver.StorageExtra {
					t.Errorf("%q has storage %q, want extra_config — it has no column", f.Name, f.Storage)
				}
			case "username", "password", "es_api_key":
				if f.Storage != driver.StorageColumn {
					t.Errorf("%q has storage %q, want column — credentials are stored encrypted", f.Name, f.Storage)
				}
			}
		}
		return
	}
	t.Fatal("elasticsearch is not among the registered types")
}

// TestDatasourceTypes_PlaceholderStyleMatchesTheDriver pins that the style is
// derived from what the driver emits, not declared beside it.
func TestDatasourceTypes_PlaceholderStyleMatchesTheDriver(t *testing.T) {
	want := map[string]driver.PlaceholderStyle{
		"mysql":         driver.PlaceholderPositional,
		"sqlite":        driver.PlaceholderPositional,
		"postgresql":    driver.PlaceholderNumbered,
		"mongodb":       driver.PlaceholderNone,
		"elasticsearch": driver.PlaceholderNone,
	}

	types, err := driver.DatasourceTypes()
	if err != nil {
		t.Fatalf("DatasourceTypes: %v", err)
	}
	for _, info := range types {
		expected, ok := want[info.Type]
		if !ok {
			continue
		}
		if info.PlaceholderStyle != expected {
			t.Errorf("%s placeholder style = %q, want %q",
				info.Type, info.PlaceholderStyle, expected)
		}
	}
}
