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
	d := NewWithClient(newUnconnectedClient(t))

	_, err := d.ExecuteQuery(context.Background(), "testdb", "not valid json", 100)
	if err == nil {
		t.Fatal("expected a parse error for a malformed command body")
	}
	if !strings.Contains(err.Error(), "parse mongodb command") {
		t.Errorf("error = %v, want it to name the parse failure", err)
	}
}

func TestExecuteQuery_RequiresDatabase(t *testing.T) {
	d := NewWithClient(newUnconnectedClient(t))

	_, err := d.ExecuteQuery(context.Background(), "", `{"operation":"find","collection":"users"}`, 100)
	if err == nil {
		t.Fatal("expected an error when no database is given")
	}
	if !strings.Contains(err.Error(), "database name is required") {
		t.Errorf("error = %v, want it to name the missing database", err)
	}
}

func TestExecuteQuery_RequiresCollection(t *testing.T) {
	d := NewWithClient(newUnconnectedClient(t))

	_, err := d.ExecuteQuery(context.Background(), "testdb", `{"operation":"find"}`, 100)
	if err == nil {
		t.Fatal("expected an error when no collection is given")
	}
	if !strings.Contains(err.Error(), "collection name is required") {
		t.Errorf("error = %v, want it to name the missing collection", err)
	}
}
