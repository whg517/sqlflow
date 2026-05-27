package connpool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestEsPoolKey 验证 ES 连接缓存 key 生成的唯一性。
func TestEsPoolKey(t *testing.T) {
	tests := []struct {
		name string
		dsID int64
		urls []string
		want string
	}{
		{
			name: "单节点",
			dsID: 1,
			urls: []string{"http://localhost:9200"},
			want: "es:1:http://localhost:9200",
		},
		{
			name: "多节点排序",
			dsID: 2,
			urls: []string{"http://node2:9200", "http://node1:9200"},
			want: "es:2:http://node1:9200,http://node2:9200",
		},
		{
			name: "不同数据源 ID 不同 key",
			dsID: 3,
			urls: []string{"http://localhost:9200"},
			want: "es:3:http://localhost:9200",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := esPoolKey(tt.dsID, tt.urls)
			if got != tt.want {
				t.Errorf("esPoolKey() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestEsPoolKeyConsistent 验证相同输入产生相同 key。
func TestEsPoolKeyConsistent(t *testing.T) {
	urls := []string{"http://c:9200", "http://a:9200", "http://b:9200"}
	key1 := esPoolKey(1, urls)
	key2 := esPoolKey(1, urls)
	if key1 != key2 {
		t.Errorf("esPoolKey 不一致: %q vs %q", key1, key2)
	}
}

// TestRemoveElasticsearch 验证 ES 连接缓存清理。
func TestRemoveElasticsearch(t *testing.T) {
	m := &Manager{}

	// 注入测试缓存
	m.esPools.Store("es:1:http://localhost:9200", "client1")
	m.esPools.Store("es:1:http://localhost:9201", "client2")
	m.esPools.Store("es:2:http://localhost:9200", "other_client")

	// 清理数据源 1 的缓存
	m.RemoveElasticsearch(1)

	// 验证清理结果
	if _, ok := m.esPools.Load("es:1:http://localhost:9200"); ok {
		t.Error("es:1:http://localhost:9200 应该被清除")
	}
	if _, ok := m.esPools.Load("es:1:http://localhost:9201"); ok {
		t.Error("es:1:http://localhost:9201 应该被清除")
	}
	if _, ok := m.esPools.Load("es:2:http://localhost:9200"); !ok {
		t.Error("es:2:http://localhost:9200 不应该被清除")
	}
}

// mockESServer 创建一个模拟 Elasticsearch 集群的 HTTP server。
// ES Go client v8 要求响应头包含 X-Elastic-Product: Elasticsearch。
func mockESServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Elastic-Product", "Elasticsearch")
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/" && r.Method == "HEAD" {
			w.WriteHeader(http.StatusOK)
			return
		}
		// _search 端点 mock
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"took":0,"hits":{"total":{"value":0},"hits":[]}}`))
	}))
}

// TestElasticsearchPingWithMockServer 使用 mock HTTP server 测试 Elasticsearch ping。
func TestElasticsearchPingWithMockServer(t *testing.T) {
	server := mockESServer()
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 测试无认证
	err := ElasticsearchPing(ctx, []string{server.URL}, "none", "", "", "", false)
	if err != nil {
		t.Errorf("ElasticsearchPing(none) 失败: %v", err)
	}

	// 测试 basic auth（mock server 不校验凭据）
	err = ElasticsearchPing(ctx, []string{server.URL}, "basic", "admin", "pass", "", false)
	if err != nil {
		t.Errorf("ElasticsearchPing(basic) 失败: %v", err)
	}

	// 测试 api_key 认证
	err = ElasticsearchPing(ctx, []string{server.URL}, "api_key", "", "", "test-key", false)
	if err != nil {
		t.Errorf("ElasticsearchPing(api_key) 失败: %v", err)
	}

	// 测试默认 authType（兼容：有用户名时按 basic 处理）
	err = ElasticsearchPing(ctx, []string{server.URL}, "", "user", "pwd", "", false)
	if err != nil {
		t.Errorf("ElasticsearchPing(default) 失败: %v", err)
	}
}

// TestElasticsearchPingError 测试连接失败场景。
func TestElasticsearchPingError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 连接不存在的地址
	err := ElasticsearchPing(ctx, []string{"http://127.0.0.1:1"}, "none", "", "", "", false)
	if err == nil {
		t.Error("连接失败时应该返回错误")
	}
}

// TestGetElasticsearchCaching 测试 ES 客户端缓存机制。
func TestGetElasticsearchCaching(t *testing.T) {
	server := mockESServer()
	defer server.Close()

	m := &Manager{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	urls := []string{server.URL}

	// 第一次获取：创建新客户端
	client1, err := m.GetElasticsearch(ctx, 1, urls, "none", "", "", "", false)
	if err != nil {
		t.Fatalf("GetElasticsearch 第一次失败: %v", err)
	}

	// 第二次获取：应该返回缓存
	client2, err := m.GetElasticsearch(ctx, 1, urls, "none", "", "", "", false)
	if err != nil {
		t.Fatalf("GetElasticsearch 第二次失败: %v", err)
	}

	if client1 != client2 {
		t.Error("两次获取应该返回相同的客户端实例（缓存）")
	}

	// 清理缓存
	m.RemoveElasticsearch(1)
}
