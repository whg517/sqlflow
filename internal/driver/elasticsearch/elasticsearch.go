// Package elasticsearch implements the Driver interface for Elasticsearch data sources.
package elasticsearch

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"

	"github.com/whg517/sqlflow/internal/driver"
	"github.com/whg517/sqlflow/internal/platform/sqlparser"
)

// defaultRowLimit bounds a query whose caller passed no limit, matching the
// other drivers rather than Elasticsearch's own default of 10.
const defaultRowLimit = 1000

func init() {
	driver.Register("elasticsearch", func() driver.Driver { return &ESDriver{} })
}

// ESDriver implements driver.Driver for Elasticsearch.
type ESDriver struct {
	client *es.Client

	// defaultIndex is the datasource's configured index pattern, used when a
	// query omits its own index.
	defaultIndex string
}

// Compile-time proof of the contracts this driver claims.
//
// The optional interfaces are satisfied structurally, so a method that is
// renamed or never lands would otherwise only surface as a capability that
// silently reports false. These assertions turn that into a build failure.
var (
	_ driver.Driver          = (*ESDriver)(nil)
	_ driver.ConfigValidator = (*ESDriver)(nil)
	_ driver.ConfigDecoder   = (*ESDriver)(nil)
	// MetadataBrowser was missing from this block while ListDatabases, ListTables
	// and GetColumns were all implemented — the only incomplete assertion block of
	// the five drivers, and precisely the drift the convention exists to catch.
	// It was covered only by a test, which is a weaker thing than a build failure.
	_ driver.MetadataBrowser = (*ESDriver)(nil)
)

// Type returns "elasticsearch".
func (d *ESDriver) Type() string { return "elasticsearch" }

// DecodeConfig reads Elasticsearch's settings out of extra_config.
//
// These lived in six dedicated columns — es_urls, es_auth_type, es_api_key,
// es_index_pattern, es_verify_certs, es_version — which reached the driver
// through a `switch dsType` in the shared config builder, a hand-written key
// switch in the datasource adapter, and a field on every request struct. None
// of those layers knew what the values meant; this one does.
//
// verify_certs defaults to on. A caller that says nothing gets certificate
// verification, and turning it off stays expressible — but only by saying so.
func (d *ESDriver) DecodeConfig(cfg *driver.Config, extra map[string]interface{}) error {
	cfg.Extra["urls"] = decodeURLs(extra["urls"])

	if v, ok := extra["auth_type"].(string); ok {
		cfg.Extra["auth_type"] = v
	} else {
		cfg.Extra["auth_type"] = "none"
	}

	verifyCerts := true
	if v, ok := extra["verify_certs"].(bool); ok {
		verifyCerts = v
	}
	cfg.Extra["verify_certs"] = verifyCerts

	if v, ok := extra["index_pattern"].(string); ok && v != "" {
		cfg.Extra["index_pattern"] = v
	}
	return nil
}

// decodeURLs accepts either a JSON array or the comma-separated string the
// es_urls column held, so a datasource written before the migration and one
// written after both resolve to the same list.
func decodeURLs(v interface{}) []string {
	switch value := v.(type) {
	case []interface{}:
		urls := make([]string, 0, len(value))
		for _, item := range value {
			if s, ok := item.(string); ok {
				if s = strings.TrimSpace(s); s != "" {
					urls = append(urls, s)
				}
			}
		}
		return urls
	case string:
		return trimNonEmpty(strings.Split(value, ","))
	default:
		return nil
	}
}

// QueryForm declares how read queries are composed for this data source.
func (d *ESDriver) QueryForm() driver.QueryForm { return driver.QueryFormDSL }

// trimNonEmpty trims each entry and drops the blanks. A whitespace-only entry
// is not an address, and letting one through turns a configuration mistake into
// an opaque URL parse error.
func trimNonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// ValidateConfig enforces Elasticsearch's transport rules.
//
// A cluster must be reachable over HTTPS unless it is on a private or loopback
// address: Elasticsearch credentials travel in the request, so plaintext HTTP
// to a public host would expose them.
func (d *ESDriver) ValidateConfig(cfg *driver.Config) error {
	urls := extractURLs(cfg)
	if len(urls) == 0 {
		return fmt.Errorf("elasticsearch: 至少需要一个连接地址")
	}
	// auth_type has to name something this driver implements. Whether the
	// credentials behind it are correct is the cluster's answer, not this one —
	// but an auth_type nobody can act on is a malformed configuration, and only
	// this driver knows the set. The handler used to switch on it by name.
	switch authType, _ := cfg.Extra["auth_type"].(string); authType {
	case "", "basic", "api_key", "none":
	default:
		return fmt.Errorf("elasticsearch: 认证方式无效: %s", authType)
	}

	for _, raw := range urls {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Hostname() == "" {
			return fmt.Errorf("Elasticsearch 连接地址无效: %s", raw)
		}
		switch strings.ToLower(parsed.Scheme) {
		case "https":
			continue
		case "http":
			host := strings.ToLower(parsed.Hostname())
			ip := net.ParseIP(host)
			if host == "localhost" || (ip != nil && (ip.IsPrivate() || ip.IsLoopback())) {
				continue
			}
			return fmt.Errorf("公网 Elasticsearch 连接地址必须使用 HTTPS，当前地址 %s 使用了 HTTP", raw)
		default:
			return fmt.Errorf("Elasticsearch 连接地址必须使用 HTTP 或 HTTPS: %s", raw)
		}
	}
	return nil
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

	if cfg.Extra != nil {
		if v, ok := cfg.Extra["index_pattern"].(string); ok {
			d.defaultIndex = v
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
func (d *ESDriver) ExecuteQuery(ctx context.Context, query string, limit int) (*driver.QueryResult, error) {
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
		index = strings.TrimSpace(d.defaultIndex)
	}
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

	// The caller's limit is the platform's row cap, so it bounds the request.
	//
	// This used to ignore limit entirely: a body without a size got 100 and one
	// with a size got up to 10000, whatever the caller asked for. A query
	// service asking for 1000 rows received 100, and a user who wrote
	// "size": 10000 into the DSL received ten times what the platform allowed —
	// the same server-side cap that every other driver truncates at.
	if limit <= 0 {
		limit = defaultRowLimit
	}
	if sizeNum, ok := toFloat64(bodyMap["size"]); ok && sizeNum < float64(limit) {
		bodyMap["size"] = sizeNum
	} else {
		bodyMap["size"] = float64(limit)
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

// Elasticsearch implements no StatementExecutor. The two methods that used to
// sit here returned "not supported" and existed only because Driver demanded
// them; the type system now carries that fact.

// Parse analyzes an Elasticsearch query JSON for security rules.
func (d *ESDriver) Parse(query string) (*driver.ParseResult, error) {
	result, err := sqlparser.ParseElasticsearchDialect(query)
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
		Aggregations json.RawMessage `json:"aggregations"`
	}

	if err := json.Unmarshal(bodyBytes, &esResp); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	elapsed := time.Since(start).Milliseconds()

	// An aggregation request typically sets size:0 and carries all of its output
	// in `aggregations`. Flattening only `hits` would return an empty table and
	// silently drop the actual answer, so aggregations get their own shape and
	// are passed through untouched.
	if len(esResp.Aggregations) > 0 {
		return &driver.QueryResult{
			Shape:         driver.ShapeAggregation,
			Columns:       []string{},
			Rows:          []map[string]interface{}{},
			Total:         esResp.Hits.Total.Value,
			ExecutionTime: elapsed,
			Aggregations:  esResp.Aggregations,
		}, nil
	}

	resultRows := make([]map[string]interface{}, 0, len(esResp.Hits.Hits))
	columnSet := make(map[string]bool)

	// The limit is what the platform promised its caller, so it is enforced on
	// the way out too. Bounding the request is an optimisation; a proxy or an
	// older cluster answering with more than it was asked for must not slip
	// past the cap.
	hits := esResp.Hits.Hits
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}

	for _, hit := range hits {
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

	// Metadata columns first, then source fields in sorted order. Ranging over
	// the set directly would inherit Go's randomized map iteration and shuffle
	// the user's columns between two runs of the same query.
	source := make([]string, 0, len(columnSet))
	for k := range columnSet {
		if k != "_id" && k != "_index" && k != "_score" {
			source = append(source, k)
		}
	}
	sort.Strings(source)
	columns := append([]string{"_id", "_index", "_score"}, source...)

	return &driver.QueryResult{
		Shape:         driver.ShapeDocuments,
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
		switch raw := cfg.Extra["urls"].(type) {
		case []string:
			return trimNonEmpty(raw)
		case []interface{}:
			strs := make([]string, 0, len(raw))
			for _, u := range raw {
				if s, ok := u.(string); ok {
					strs = append(strs, s)
				}
			}
			return trimNonEmpty(strs)
		case string:
			// A single comma-separated value, as stored on the datasource.
			return trimNonEmpty(strings.Split(raw, ","))
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

// ConfigSchema declares what an Elasticsearch connection is made of.
//
// Every setting except the credentials lands in extra_config, which is where
// the server has kept them since the five dedicated columns were removed. The
// form carried those five as named fields long after the columns were gone, and
// showed or hid them by comparing the type name to "elasticsearch" in six
// places; the auth-type condition below is what replaces that.
func (d *ESDriver) ConfigSchema() []driver.ConfigField {
	basicOrKey := func(values ...string) *driver.FieldCondition {
		return &driver.FieldCondition{Field: "auth_type", Equals: values}
	}

	return []driver.ConfigField{
		{
			Name: "urls", Label: "节点地址", Kind: driver.FieldText, Required: true,
			Placeholder: "https://es1.example.com:9200, https://es2.example.com:9200",
			Help:        "多个地址用逗号分隔",
			Storage:     driver.StorageExtra,
		},
		{
			Name: "auth_type", Label: "认证方式", Kind: driver.FieldSelect,
			Default: "basic", Storage: driver.StorageExtra,
			Options: []driver.ConfigOption{
				{Value: "basic", Label: "Basic Auth"},
				{Value: "api_key", Label: "API Key"},
				{Value: "none", Label: "无认证"},
			},
		},
		{
			Name: "username", Label: "用户名", Kind: driver.FieldText, Required: true,
			Storage: driver.StorageColumn, ShowWhen: basicOrKey("basic"),
		},
		{
			Name: "password", Label: "密码", Kind: driver.FieldPassword, Required: true,
			Storage: driver.StorageColumn, Secret: true, ShowWhen: basicOrKey("basic"),
		},
		{
			Name: "es_api_key", Label: "API Key", Kind: driver.FieldPassword, Required: true,
			Storage: driver.StorageColumn, Secret: true, ShowWhen: basicOrKey("api_key"),
		},
		{
			Name: "index_pattern", Label: "索引模式", Kind: driver.FieldText,
			Placeholder: "logs-*（留空表示全部索引）", Storage: driver.StorageExtra,
		},
		{
			Name: "verify_certs", Label: "校验证书", Kind: driver.FieldSwitch,
			Default: "true", Storage: driver.StorageExtra,
			Help: "关闭后 TLS 证书不再校验，仅用于自签名环境",
		},
	}
}
