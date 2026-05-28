package coverage

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

type ParserRegistry struct {
	mu        sync.RWMutex
	providers map[ReportType]CoverageProvider
}

func NewParserRegistry() *ParserRegistry {
	return &ParserRegistry{providers: make(map[ReportType]CoverageProvider)}
}

func (r *ParserRegistry) Register(p CoverageProvider) error {
	if p == nil {
		return fmt.Errorf("coverage: cannot register nil provider")
	}
	name := p.Name()
	if name == "" {
		return fmt.Errorf("coverage: provider name cannot be empty")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("coverage: provider %q already registered", name)
	}
	r.providers[name] = p
	return nil
}

func (r *ParserRegistry) MustRegister(p CoverageProvider) {
	if err := r.Register(p); err != nil {
		panic(err)
	}
}

func (r *ParserRegistry) Lookup(reportType ReportType) (CoverageProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.providers[reportType]
	if !ok {
		return nil, fmt.Errorf("coverage: no provider registered for report type %q", reportType)
	}
	return p, nil
}

func (r *ParserRegistry) List() []ReportType {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]ReportType, 0, len(r.providers))
	for t := range r.providers {
		types = append(types, t)
	}
	return types
}

func (r *ParserRegistry) Parse(ctx context.Context, reportType ReportType, reader io.Reader) (*ParseResult, error) {
	p, err := r.Lookup(reportType)
	if err != nil {
		return nil, err
	}
	return p.Parse(ctx, reader)
}

func (r *ParserRegistry) DetectAndParse(ctx context.Context, reader io.Reader) (*ParseResult, ReportType, error) {
	r.mu.RLock()
	providers := make([]CoverageProvider, 0, len(r.providers))
	for _, p := range r.providers {
		providers = append(providers, p)
	}
	r.mu.RUnlock()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", fmt.Errorf("coverage: failed to read report data: %w", err)
	}
	for _, p := range providers {
		if detector, ok := p.(FormatDetector); ok {
			if detector.Detect(data) {
				result, err := p.Parse(ctx, newBytesReader(data))
				if err != nil {
					continue
				}
				return result, p.Name(), nil
			}
		}
	}
	return nil, "", fmt.Errorf("coverage: no provider matched the report format")
}

func ParseFileHelper(ctx context.Context, path string, parseFn func(context.Context, io.Reader) (*ParseResult, error)) (*ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("coverage: open %s: %w", path, err)
	}
	defer f.Close()
	return parseFn(ctx, f)
}

type BaseProvider struct {
	TypeName  ReportType
	Languages []Language
}

func (b BaseProvider) Name() ReportType            { return b.TypeName }
func (b BaseProvider) SupportedLanguages() []Language { return b.Languages }
func (b BaseProvider) ParseFile(ctx context.Context, path string) (*ParseResult, error) {
	return ParseFileHelper(ctx, path, func(ctx context.Context, r io.Reader) (*ParseResult, error) {
		return nil, fmt.Errorf("coverage: BaseProvider.Parse should not be called directly")
	})
}

var (
	globalRegistryOnce sync.Once
	globalRegistry     *ParserRegistry
)

func GlobalRegistry() *ParserRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = NewParserRegistry()
	})
	return globalRegistry
}

// NICE-5: init auto-registers built-in providers.
func init() {
	GlobalRegistry().MustRegister(NewLCOVProvider())
}

type bytesReader struct {
	data []byte
	pos  int
}

func newBytesReader(data []byte) *bytesReader { return &bytesReader{data: data} }
func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
