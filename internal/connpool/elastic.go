package connpool

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	es "github.com/elastic/go-elasticsearch/v8"
)

// esPoolKey 生成 Elasticsearch 连接的缓存 key。
// key 格式: es:{dsID}:{sorted_urls}
func esPoolKey(dsID int64, urls []string) string {
	sorted := make([]string, len(urls))
	copy(sorted, urls)
	sort.Strings(sorted)
	return fmt.Sprintf("es:%d:%s", dsID, strings.Join(sorted, ","))
}

// GetElasticsearch 返回一个缓存的 *es.Client，
// 如果不存在则创建新客户端并验证连接。
// authType: "basic" (用户名密码), "api_key" (API Key), "none" (无认证)
// apiKey: 已解密的 API Key（调用方负责解密）
func (m *Manager) GetElasticsearch(ctx context.Context, dsID int64, urls []string, authType, username, password, apiKey string, verifyCerts bool) (*es.Client, error) {
	key := esPoolKey(dsID, urls)

	// 快速路径：检查缓存
	if v, ok := m.esPools.Load(key); ok {
		return v.(*es.Client), nil
	}

	// 慢速路径：创建新客户端
	m.mu.Lock()
	defer m.mu.Unlock()

	// 双重检查
	if v, ok := m.esPools.Load(key); ok {
		return v.(*es.Client), nil
	}

	cfg := es.Config{
		Addresses: urls,
	}

	// 根据认证类型配置
	switch authType {
	case "api_key":
		cfg.Header = http.Header{"Authorization": {"ApiKey " + apiKey}}
	case "basic":
		cfg.Username = username
		cfg.Password = password
	case "none":
		// 无认证，ES 匿名访问
	default:
		// 兼容：未指定 auth_type 时按 basic 处理
		if username != "" {
			cfg.Username = username
			cfg.Password = password
		}
	}

	// 证书验证配置
	if !verifyCerts {
		cfg.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	client, err := es.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Elasticsearch 客户端: %w", err)
	}

	// Ping 验证连接
	pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	res, err := client.Ping(client.Ping.WithContext(pingCtx))
	if err != nil {
		return nil, fmt.Errorf("Elasticsearch ping 失败: %w", err)
	}
	_ = res.Body.Close()
	if res.IsError() {
		return nil, fmt.Errorf("Elasticsearch ping 返回错误: %s", res.Status())
	}

	m.esPools.Store(key, client)
	return client, nil
}

// RemoveElasticsearch 清理指定数据源 ID 的所有 Elasticsearch 客户端缓存。
func (m *Manager) RemoveElasticsearch(dsID int64) {
	prefix := fmt.Sprintf("es:%d:", dsID)
	m.esPools.Range(func(key, _ interface{}) bool {
		if strings.HasPrefix(key.(string), prefix) {
			m.esPools.Delete(key)
		}
		return true
	})
}

// InjectESForTest stores a pre-built client so a test can exercise index and
// field browsing without a cluster.
//
// It is the one injection hook that survived the package's cleanup, because it
// is the one with a caller: internal/datasource's ES pagination tests. The
// MySQL and MongoDB equivalents were kept alive only by tests of the pools they
// injected into, which no longer exist.
func (m *Manager) InjectESForTest(dsID int64, urls []string, client *es.Client) {
	m.esPools.Store(esPoolKey(dsID, urls), client)
}

// CachedESIDs lists the datasources with a cached client.
//
// Test support with a live caller: internal/datasource asserts that rotating a
// credential or deleting a datasource evicts the client, which is a regression
// test for a cache that used to survive both.
func (m *Manager) CachedESIDs() []int64 {
	var ids []int64
	m.esPools.Range(func(key, _ any) bool {
		var dsID int64
		if _, err := fmt.Sscanf(key.(string), "es:%d:", &dsID); err == nil {
			ids = append(ids, dsID)
		}
		return true
	})
	return ids
}
