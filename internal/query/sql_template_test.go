package query

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/whg517/sqlflow/internal/testutil"
)

func TestTemplateServiceRenderParameterizedSQL(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewTemplateService(database)
	ctx := context.Background()

	tests := []struct {
		name       string
		dbType     string
		sql        string
		params     map[string]string
		wantSQL    string
		wantParams []interface{}
	}{
		{
			name:   "mysql",
			dbType: "mysql",
			sql:    "SELECT * FROM users WHERE id = {{ id }} AND status = {{status: active}}",
			params: map[string]string{"id": "42"},
			// MySQL binds positionally: the placeholder style is the target
			// datasource's, not the platform store's.
			wantSQL:    "SELECT * FROM users WHERE id = ? AND status = ?",
			wantParams: []interface{}{"42", "active"},
		},
		{
			name:       "postgresql",
			dbType:     "postgresql",
			sql:        "SELECT * FROM users WHERE team_id = {{team}} AND enabled = {{enabled:true}}",
			params:     map[string]string{"team": "7"},
			wantSQL:    "SELECT * FROM users WHERE team_id = $1 AND enabled = $2",
			wantParams: []interface{}{"7", "true"},
		},
		{
			// SQLite binds positionally like MySQL; previously this fell through
			// to a default branch rather than being stated anywhere.
			name:       "sqlite",
			dbType:     "sqlite",
			sql:        "SELECT * FROM users WHERE id = {{id}}",
			params:     map[string]string{"id": "9"},
			wantSQL:    "SELECT * FROM users WHERE id = ?",
			wantParams: []interface{}{"9"},
		},
		{
			// MongoDB has nothing to bind to, so values are escaped and inlined.
			name:       "mongodb",
			dbType:     "mongodb",
			sql:        `{"collection":"users","filter":{"name":"{{name}}"}}`,
			params:     map[string]string{"name": "a b&c"},
			wantSQL:    `{"collection":"users","filter":{"name":"a+b%26c"}}`,
			wantParams: []interface{}{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			template, err := svc.CreateTemplate(ctx, 1, "render-"+test.name, "", test.sql, test.dbType, "query", false)
			if err != nil {
				t.Fatalf("CreateTemplate: %v", err)
			}

			result, err := svc.RenderTemplateForUser(ctx, template.ID, 1, test.params)
			if err != nil {
				t.Fatalf("RenderTemplateForUser: %v", err)
			}
			if result.RenderedSQL != test.wantSQL {
				t.Fatalf("RenderedSQL = %q, want %q", result.RenderedSQL, test.wantSQL)
			}
			if !reflect.DeepEqual(result.ParamValues, test.wantParams) {
				t.Fatalf("ParamValues = %#v, want %#v", result.ParamValues, test.wantParams)
			}
		})
	}
}

func TestTemplateServiceRenderRejectsMissingParameter(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewTemplateService(database)
	template, err := svc.CreateTemplate(context.Background(), 1, "missing-param", "", "SELECT * FROM users WHERE id = {{id}}", "mysql", "query", false)
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	_, err = svc.RenderTemplateForUser(context.Background(), template.ID, 1, map[string]string{})
	if !errors.Is(err, ErrTemplateParamMissing) {
		t.Fatalf("RenderTemplateForUser error = %v, want ErrTemplateParamMissing", err)
	}
}

func TestTemplateServicePrivateTemplateAccess(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewTemplateService(database)
	template, err := svc.CreateTemplate(context.Background(), 1, "private-access", "", "SELECT 1", "mysql", "query", false)
	if err != nil {
		t.Fatalf("CreateTemplate: %v", err)
	}

	if _, err := svc.GetTemplateForUser(context.Background(), template.ID, 1); err != nil {
		t.Fatalf("owner access failed: %v", err)
	}
	if _, err := svc.GetTemplateForUser(context.Background(), template.ID, 2); !errors.Is(err, ErrTemplateNotFound) {
		t.Fatalf("other user error = %v, want ErrTemplateNotFound", err)
	}
}

func TestTemplateServiceListCategoryFiltersOwnedTemplates(t *testing.T) {
	database := testutil.NewDB(t)
	svc := NewTemplateService(database)
	ctx := context.Background()
	if _, err := svc.CreateTemplate(ctx, 1, "query-template", "", "SELECT 1", "mysql", "query", false); err != nil {
		t.Fatalf("create query template: %v", err)
	}
	if _, err := svc.CreateTemplate(ctx, 1, "dml-template", "", "UPDATE users SET active = 1", "mysql", "dml", false); err != nil {
		t.Fatalf("create dml template: %v", err)
	}

	templates, total, err := svc.ListTemplates(ctx, 1, "query", 1, 20)
	if err != nil {
		t.Fatalf("ListTemplates: %v", err)
	}
	if total != 1 || len(templates) != 1 || templates[0].Name != "query-template" {
		t.Fatalf("filtered templates = %#v, total = %d", templates, total)
	}
}
