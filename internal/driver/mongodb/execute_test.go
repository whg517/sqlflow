package mongodb

import (
	"context"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Command validation used to be covered through QueryService's connpool helper.
// It belongs to the driver, so it is pinned here. A client built from a URI is
// enough: these checks run before any network round trip.
func newUnconnectedClient(t *testing.T) *mongo.Client {
	t.Helper()
	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017")) //nolint:staticcheck // validation runs before connecting
	if err != nil {
		t.Fatalf("build mongo client: %v", err)
	}
	return client
}

func TestExecuteQuery_RejectsInvalidCommandJSON(t *testing.T) {
	d := NewWithClient(newUnconnectedClient(t), "testdb")

	_, err := d.ExecuteQuery(context.Background(), "not valid json", 100)
	if err == nil {
		t.Fatal("expected a parse error for a malformed command body")
	}
	if !strings.Contains(err.Error(), "parse mongodb command") {
		t.Errorf("error = %v, want it to name the parse failure", err)
	}
}

// TestExecuteQuery_RequiresDatabase covers a datasource saved without one.
//
// The scope now comes from the connection rather than from the caller, so the
// only way to reach this is a MongoDB datasource whose row and URI both name no
// database. Refusing beats silently reading Mongo's "test" default.
func TestExecuteQuery_RequiresDatabase(t *testing.T) {
	d := NewWithClient(newUnconnectedClient(t), "")

	_, err := d.ExecuteQuery(context.Background(), `{"operation":"find","collection":"users"}`, 100)
	if err == nil {
		t.Fatal("expected an error when the datasource has no database")
	}
	if !strings.Contains(err.Error(), "数据源未配置数据库") {
		t.Errorf("error = %v, want it to name the missing database", err)
	}
}

func TestExecuteQuery_RequiresCollection(t *testing.T) {
	d := NewWithClient(newUnconnectedClient(t), "testdb")

	_, err := d.ExecuteQuery(context.Background(), `{"operation":"find"}`, 100)
	if err == nil {
		t.Fatal("expected an error when no collection is given")
	}
	if !strings.Contains(err.Error(), "collection name is required") {
		t.Errorf("error = %v, want it to name the missing collection", err)
	}
}
