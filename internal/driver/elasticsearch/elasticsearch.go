// Package elasticsearch implements the Driver interface for Elasticsearch data sources.
package elasticsearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/pkg/sqlparser"
)

func init() {
	driver.Register("elasticsearch", func() driver.Driver { return &ESDriver{} })
}

// ESDriver implements driver.Driver for Elasticsearch.
type ESDriver struct {
	client *es.Client
}

// Type returns "elasticsearch".
func (d *ESDriver) Type() string { return "elasticsearch" }

// Capabilities declares Elasticsearch's capability set.
// ES does not support CapTicketExec (no DML/DDL), CapSQLParse (no SQL syntax),
// or CapTableLevelPermission (no row-level access control).
func (d *ESDriver) Capabilities() driver.CapabilitySet {
	return driver.CapabilitySet(
		driver.CapQuery |
			driver.CapMetadata |
			driver.CapFieldMasking |
			driver.CapExport,
	)
}

// Connect establishes a connection to the Elasticsearch cluster.
func (d *ESDriver) Connect(ctx context.Context, cfg *driver.Config) error {
	urls := extractURLs(cfg)
	if len(urls) == 0 {
		return fmt.Errorf("elasticsearch: connection URLs are required")
	}

	esConfig := es.Config{Addresses: urls}

	// Configure authentication
	authType := "none"
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["auth_type"].(string); ok {
			authType = v
		}
	}

	switch authType {
	case "api_key":
		if apiKey, ok := cfg.Extra["api_key"].(string); ok {
			esConfig.Header = http.Header{"Authorization": {"ApiKey " + apiKey}}
		}
	case "basic":
		esConfig.Username = cfg.Username
		esConfig.Password = cfg.Password
	case "none":
		// Anonymous access
	default:
		if cfg.Username != "" {
			esConfig.Username = cfg.Username
			esConfig.Password = cfg.Password
		}
	}

	// TLS configuration
	verifyCerts := true
	if cfg.Extra != nil {
		if v, ok := cfg.Extra["verify_certs"].(bool); ok {
			verifyCerts = v
		}
	}
	if !verifyCerts {
		esConfig.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	client, err := es.NewClient(esConfig)
	if err != nil {
		return fmt.Errorf("create elasticsearch client: %w", err)
	}

	// Ping verification
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := client.Ping(client.Ping.WithContext(pingCtx))
	if err != nil {
		return fmt.Errorf("elasticsearch ping failed: %w", err)
	}
	_ = res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch ping returned error: %s", res.Status())
	}

	d.client = client
	return nil
}

// Close releases resources held by the ES client.
// The ES client does not have an explicit Close method; this is a no-op.
func (d *ESDriver) Close() error {
	d.client = nil
	return nil
}

// Ping verifies the Elasticsearch connection is alive.
func (d *ESDriver) Ping(ctx context.Context) error {
	if d.client == nil {
		return fmt.Errorf("elasticsearch: not connected")
	}

	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := d.client.Ping(d.client.Ping.WithContext(pingCtx))
	if err != nil {
		return fmt.Errorf("elasticsearch ping failed: %w", err)
	}
	_ = res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch ping returned error: %s", res.Status())
	}
	return nil
}

// ListDatabases returns index names. For Elasticsearch, there is no
// concept of "databases" — we return index names as the closest equivalent.
func (d *ESDriver) ListDatabases(ctx context.Context) ([]string, error) {
	if d.client == nil {
		return nil, fmt.Errorf("elasticsearch: not connected")
	}

	res, err := d.client.Cat.Indices(
		d.client.Cat.Indices.WithContext(ctx),
		d.client.Cat.Indices.WithFormat("json"),
		d.client.Cat.Indices.WithH("index"),
	)
	if err != nil {
		return nil, fmt.Errorf("list indices: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch error (%d)", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	type catIndexEntry struct {
		Index string `json:"index"`
	}
	var entries []catIndexEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse cat indices: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if strings.HasPrefix(entry.Index, ".") {
			continue
		}
		names = append(names, entry.Index)
	}
	sort.Strings(names)
	return names, nil
}

// ListTables returns index names (treated as "tables").
func (d *ESDriver) ListTables(ctx context.Context, database string) ([]driver.TableInfo, error) {
	if d.client == nil {
		return nil, fmt.Errorf("elasticsearch: not connected")
	}

	indexPattern := database
	if indexPattern == "" {
		indexPattern = "*"
	}

	res, err := d.client.Cat.Indices(
		d.client.Cat.Indices.WithContext(ctx),
		d.client.Cat.Indices.WithFormat("json"),
		d.client.Cat.Indices.WithIndex(indexPattern),
		d.client.Cat.Indices.WithH("index"),
	)
	if err != nil {
		return nil, fmt.Errorf("list indices: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch error (%d)", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	type catIndexEntry struct {
		Index string `json:"index"`
	}
	var entries []catIndexEntry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("parse cat indices: %w", err)
	}

	tables := make([]driver.TableInfo, 0, len(entries))
	for _, entry := range entries {
		tables = append(tables, driver.TableInfo{Name: entry.Index})
	}
	sort.Slice(tables, func(i, j int) bool {
		return tables[i].Name < tables[j].Name
	})
	return tables, nil
}

// GetColumns returns field mappings for an index.
func (d *ESDriver) GetColumns(ctx context.Context, database, index string) ([]driver.ColumnInfo, error) {
	if d.client == nil {
		return nil, fmt.Errorf("elasticsearch: not connected")
	}
	if index == "" {
		return nil, fmt.Errorf("elasticsearch: index name is required")
	}

	// Use PerformRequest with the ES low-level API for GetMapping
	res, err := d.client.Indices.GetMapping(
		d.client.Indices.GetMapping.WithContext(ctx),
		d.client.Indices.GetMapping.WithIndex(index),
	)
	if err != nil {
		return nil, fmt.Errorf("get mapping: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch error (%d)", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	// Parse mapping to extract field names and types.
	// Response format: {"index_name": {"mappings": {"properties": {"field": {"type": "text"}}}}}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse mapping: %w", err)
	}

	columns := make([]driver.ColumnInfo, 0)
	for _, idxRaw := range raw {
		var idx struct {
			Mappings struct {
				Properties map[string]struct {
					Type string `json:"type"`
				} `json:"properties"`
			} `json:"mappings"`
		}
		if err := json.Unmarshal(idxRaw, &idx); err != nil {
			continue
		}
		for fieldName, fieldProps := range idx.Mappings.Properties {
			columns = append(columns, driver.ColumnInfo{
				Name: fieldName,
				Type: fieldProps.Type,
			})
		}
	}

	if len(columns) == 0 {
		return nil, nil
	}
	return columns, nil
}

// ExecuteQuery executes a search or count query against an Elasticsearch index.
// The query string is a JSON body: {"index": "my-index-*", "operation": "search|count", "body": {"query": {...}}}
func (d *ESDriver) ExecuteQuery(ctx context.Context, database string, query string, limit int) (*driver.QueryResult, error) {
	if d.client == nil {
		return nil, fmt.Errorf("elasticsearch: not connected")
	}

	var req sqlparser.ESQueryRequest
	if err := json.Unmarshal([]byte(query), &req); err != nil {
		return nil, fmt.Errorf("parse elasticsearch query: %w", err)
	}

	op := strings.ToLower(strings.TrimSpace(req.Operation))
	if op == "" {
		op = "search"
	}
	if op != "search" && op != "count" {
		return nil, fmt.Errorf("unsupported ES operation: %s, only search and count allowed", op)
	}

	index := strings.TrimSpace(req.Index)
	if index == "" {
		return nil, fmt.Errorf("elasticsearch query must specify an index")
	}

	// Parse and sanitize body
	var bodyMap map[string]interface{}
	if len(req.Body) > 0 {
		if err := json.Unmarshal(req.Body, &bodyMap); err != nil {
			return nil, fmt.Errorf("parse elasticsearch body: %w", err)
		}
	}
	if bodyMap == nil {
		bodyMap = make(map[string]interface{})
	}

	// Inject timeout
	bodyMap["timeout"] = "30s"

	// Enforce size limit
	const esMaxSize = 10000
	const esDefaultSize = 100
	if sizeVal, ok := bodyMap["size"]; ok {
		if sizeNum, ok := toFloat64(sizeVal); ok && sizeNum > float64(esMaxSize) {
			bodyMap["size"] = float64(esMaxSize)
		}
	} else {
		bodyMap["size"] = float64(esDefaultSize)
	}

	bodyJSON, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("serialize request: %w", err)
	}

	queryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	start := time.Now()

	switch op {
	case "count":
		return d.executeCount(queryCtx, index, bodyJSON, start)
	default: // "search"
		return d.executeSearch(queryCtx, index, bodyJSON, start, limit)
	}
}

// ExecuteStatement is not supported for Elasticsearch (read-only).
func (d *ESDriver) ExecuteStatement(ctx context.Context, database string, stmt string) (*driver.StatementResult, error) {
	return nil, fmt.Errorf("elasticsearch: statement execution is not supported (read-only data source)")
}

// ExecuteStatements is not supported for Elasticsearch (read-only data source).
func (d *ESDriver) ExecuteStatements(ctx context.Context, database string, statements []string) ([]driver.StatementResult, error) {
	return nil, fmt.Errorf("elasticsearch: statement execution is not supported (read-only data source)")
}

// Parse analyzes an Elasticsearch query JSON for security rules.
func (d *ESDriver) Parse(query string) (*driver.ParseResult, error) {
	result, err := sqlparser.ParseSQL(query, "elasticsearch")
	if err != nil {
		return nil, err
	}

	pr := &driver.ParseResult{
		Operation: string(result.Operation),
		Targets:   result.Tables,
		Warnings:  result.Warnings,
	}

	if result.IsBlocked {
		pr.IsBlocked = true
		pr.BlockReason = result.BlockReason
	}

	pr.RiskLevel = string(result.RiskLevel)

	return pr, nil
}

// ---------------------------------------------------------------------------
// ES query execution helpers
// ---------------------------------------------------------------------------

func (d *ESDriver) executeSearch(ctx context.Context, index string, bodyJSON []byte, start time.Time, limit int) (*driver.QueryResult, error) {
	res, err := d.client.Search(
		d.client.Search.WithContext(ctx),
		d.client.Search.WithIndex(index),
		d.client.Search.WithBody(bytes.NewReader(bodyJSON)),
	)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("查询超时")
		}
		return nil, fmt.Errorf("execute elasticsearch _search: %w", err)
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch error (%d): %s", res.StatusCode, string(bodyBytes))
	}

	var esResp struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				Index  string                 `json:"_index"`
				ID     string                 `json:"_id"`
				Score  *float64               `json:"_score"`
				Source map[string]interface{} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.Unmarshal(bodyBytes, &esResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	resultRows := make([]map[string]interface{}, 0, len(esResp.Hits.Hits))
	columnSet := make(map[string]bool)

	for _, hit := range esResp.Hits.Hits {
		row := make(map[string]interface{})
		row["_id"] = hit.ID
		row["_index"] = hit.Index
		if hit.Score != nil {
			row["_score"] = *hit.Score
		}
		for k, v := range hit.Source {
			row[k] = v
		}
		for k := range row {
			columnSet[k] = true
		}
		resultRows = append(resultRows, row)
	}

	// Column order: _id, _index, _score, then remaining keys
	columns := []string{"_id", "_index", "_score"}
	for k := range columnSet {
		if k != "_id" && k != "_index" && k != "_score" {
			columns = append(columns, k)
		}
	}

	elapsed := time.Since(start).Milliseconds()

	return &driver.QueryResult{
		Columns:       columns,
		Rows:          resultRows,
		Total:         esResp.Hits.Total.Value,
		ExecutionTime: elapsed,
	}, nil
}

func (d *ESDriver) executeCount(ctx context.Context, index string, bodyJSON []byte, start time.Time) (*driver.QueryResult, error) {
	res, err := d.client.Count(
		d.client.Count.WithContext(ctx),
		d.client.Count.WithIndex(index),
		d.client.Count.WithBody(bytes.NewReader(bodyJSON)),
	)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("查询超时")
		}
		return nil, fmt.Errorf("execute elasticsearch _count: %w", err)
	}
	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch error (%d): %s", res.StatusCode, string(bodyBytes))
	}

	var countResp struct {
		Count int64 `json:"count"`
	}
	if err := json.Unmarshal(bodyBytes, &countResp); err != nil {
		return nil, fmt.Errorf("parse count response: %w", err)
	}

	elapsed := time.Since(start).Milliseconds()

	return &driver.QueryResult{
		Columns:       []string{"count"},
		Rows:          []map[string]interface{}{{"count": countResp.Count}},
		Total:         countResp.Count,
		ExecutionTime: elapsed,
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func extractURLs(cfg *driver.Config) []string {
	if cfg.Extra != nil {
		if urls, ok := cfg.Extra["urls"].([]interface{}); ok {
			result := make([]string, 0, len(urls))
			for _, u := range urls {
				if s, ok := u.(string); ok && s != "" {
					result = append(result, s)
				}
			}
			return result
		}
		if s, ok := cfg.Extra["urls"].(string); ok && s != "" {
			return []string{s}
		}
	}

	// Fallback: build URL from host/port
	if cfg.Host != "" && cfg.Port > 0 {
		protocol := "http"
		if cfg.SSLMode == "require" || cfg.SSLMode == "verify-full" {
			protocol = "https"
		}
		return []string{fmt.Sprintf("%s://%s:%d", protocol, cfg.Host, cfg.Port)}
	}

	return nil
}

func toFloat64(v interface{}) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}
