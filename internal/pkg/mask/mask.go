package mask

// Engine applies masking rules to data (placeholder).
type Engine interface {
	Apply(data map[string]interface{}, rules []string) map[string]interface{}
}
