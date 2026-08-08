package mongodb

import (
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/whg517/sqlflow/internal/driver"
)

// NewWithClient wraps an existing mongo.Client as a connected driver bound to
// the given database.
//
// It exists so tests can reach the command-parsing and validation paths without
// a live server: a client built from a URI is usable for those checks even
// though no connection has been established. Production code must go through
// Connect, which also validates configuration, derives the scope and manages
// the client lifetime.
func NewWithClient(client *mongo.Client, database string) driver.Driver {
	return &MongoDBDriver{client: client, database: database}
}
