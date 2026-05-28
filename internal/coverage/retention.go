package coverage

import (
	"context"
	"log"
	"time"
)

type RetentionPolicy struct {
	store             *Store
	rawRetentionDays  int
	fileRetentionDays int
	interval          time.Duration
	stopCh            chan struct{}
}

func NewRetentionPolicy(store *Store) *RetentionPolicy {
	return &RetentionPolicy{store: store, rawRetentionDays: 90, fileRetentionDays: 90, interval: 24 * time.Hour, stopCh: make(chan struct{})}
}
func (r *RetentionPolicy) WithRawRetention(days int) *RetentionPolicy {
	if days > 0 { r.rawRetentionDays = days }; return r
}
func (r *RetentionPolicy) WithFileRetention(days int) *RetentionPolicy {
	if days > 0 { r.fileRetentionDays = days }; return r
}
func (r *RetentionPolicy) WithInterval(d time.Duration) *RetentionPolicy {
	if d > 0 { r.interval = d }; return r
}
func (r *RetentionPolicy) Start(ctx context.Context) {
	go func() {
		r.runCleanup(ctx)
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done(): return
			case <-r.stopCh: return
			case <-ticker.C: r.runCleanup(ctx)
			}
		}
	}()
	log.Printf("[coverage] retention policy started (raw=%d days, file=%d days)", r.rawRetentionDays, r.fileRetentionDays)
}
func (r *RetentionPolicy) Stop() { close(r.stopCh) }
func (r *RetentionPolicy) runCleanup(ctx context.Context) {
	n, err := r.store.PurgeRawData(ctx, r.rawRetentionDays, r.fileRetentionDays)
	if err != nil { log.Printf("[coverage] retention cleanup error: %v", err); return }
	if n > 0 { log.Printf("[coverage] retention cleanup: purged %d records", n) }
}
