package sqlparser

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MongoOperation represents a MongoDB operation type.
type MongoOperation string

const (
	MongoOpFind      MongoOperation = "find"
	MongoOpAggregate MongoOperation = "aggregate"
	MongoOpUpdate    MongoOperation = "update"
	MongoOpDelete    MongoOperation = "delete"
	MongoOpUnknown   MongoOperation = "unknown"
)

// MongoParseResult holds the parsed result of a MongoDB command body.
type MongoParseResult struct {
	Operation         MongoOperation
	Collection        string
	HasFilter         bool
	PipelineStages    []string
	HasDangerousStage bool
	IsMulti           bool // true for updateMany/deleteMany
	HasEmptyFilter    bool // true if filter is {} or absent
}

// Allowed aggregation stages whitelist.
// These are safe read-only / transformation stages that do not modify data or collections.
var allowedAggStages = map[string]bool{
	// Filtering & matching
	"$match": true,

	// Grouping & accumulation
	"$group": true,
	"$count": true,
	"$bucket": true,
	"$bucketAuto": true,
	"$densify": true,
	"$fill": true,

	// Projection & shaping
	"$project": true,
	"$addFields": true,
	"$set": true,       // $set is an alias for $addFields (MongoDB 4.2+)
	"$unset": true,     // $unset removes fields (safe, read-only transform)
	"$unsetField": true,
	"$setField": true,
	"$replaceRoot": true,
	"$replaceWith": true, // alias for $replaceRoot
	"$setWindowFields": true,

	// Sorting & pagination
	"$sort": true,
	"$limit": true,
	"$skip": true,

	// Unwinding arrays
	"$unwind": true,

	// Joining collections (read-only lookup)
	"$lookup": true,
	"$graphLookup": true,

	// Faceted search / multiple pipelines
	"$facet": true,

	// Statistical / geo
	"$geoNear": true,
	"$near": true,
	"$nearSphere": true,
	"$sample": true,
	"$search": true,
	"$searchMeta": true,
	"$vectorSearch": true,

	// Date / expression helpers (used inside stages, but also valid as stages in some contexts)
	"$documents": true,
	"$sortArray": true,
	"$reduce": true,
	"$map": true,
	"$filter": true,

	// Collation / plan hints (meta directives)
	"$collStats": true,
	"$indexStats": true,
	"$planCacheStats": true,
}

// blockedAggStages are explicitly blocked stages that would modify data or pose security risk.
// Even if someone tries to use them, they will be caught as dangerous.
var blockedAggStages = map[string]bool{
	"$out":            true, // writes to collection
	"$merge":          true, // writes to collection
	"$currentOp":      true, // exposes system operations
	"$listLocalSessions": true,
	"$listSessions":   true,
	"$changeStream":   true,
}

// ParseMongo parses a MongoDB command JSON body and returns structured result.
func ParseMongo(body string) (*MongoParseResult, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, fmt.Errorf("empty MongoDB command body")
	}

	var m map[string]interface{}
	if err := json.Unmarshal([]byte(body), &m); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	result := &MongoParseResult{}

	// Extract collection name
	if coll, ok := m["collection"].(string); ok {
		result.Collection = coll
	}

	// Determine operation
	result.Operation = determineMongoOp(m)

	// Check for multi flag
	if multi, ok := m["multi"].(bool); ok {
		result.IsMulti = multi
	}

	// Check filter presence
	result.HasFilter, result.HasEmptyFilter = checkMongoFilter(m)

	// Process pipeline stages for aggregate
	if pipeline, ok := m["pipeline"].([]interface{}); ok {
		for _, stage := range pipeline {
			if stageMap, ok := stage.(map[string]interface{}); ok {
				for key := range stageMap {
					result.PipelineStages = append(result.PipelineStages, key)
					// Check if stage is in the explicitly blocked list
					if blockedAggStages[key] {
						result.HasDangerousStage = true
					} else if !allowedAggStages[key] {
						// Unknown stages are also treated as dangerous for safety
						result.HasDangerousStage = true
					}
				}
			}
		}
	}

	return result, nil
}

// determineMongoOp determines the MongoDB operation from the parsed JSON map.
func determineMongoOp(m map[string]interface{}) MongoOperation {
	// Explicit operation field takes priority
	if op, ok := m["operation"].(string); ok {
		switch strings.ToLower(op) {
		case "find":
			return MongoOpFind
		case "aggregate":
			return MongoOpAggregate
		case "updateone", "updatemany", "update":
			return MongoOpUpdate
		case "deleteone", "deletemany", "delete":
			return MongoOpDelete
		}
	}

	// Infer from keys
	if _, ok := m["pipeline"]; ok {
		return MongoOpAggregate
	}
	if _, ok := m["update"]; ok {
		return MongoOpUpdate
	}
	if _, ok := m["filter"]; ok {
		if _, hasUpdate := m["update"]; hasUpdate {
			return MongoOpUpdate
		}
		return MongoOpFind
	}

	return MongoOpFind
}

// checkMongoFilter checks filter presence and emptiness.
func checkMongoFilter(m map[string]interface{}) (hasFilter bool, isEmpty bool) {
	f, exists := m["filter"]
	if !exists || f == nil {
		return false, true
	}
	if filterMap, ok := f.(map[string]interface{}); ok {
		return true, len(filterMap) == 0
	}
	return true, false
}
