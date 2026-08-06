package notify

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"testing"
)

// seedDeadLetterWebhook creates a webhook to hang dead letters off.
func seedDeadLetterWebhook(t *testing.T, svc *FeishuService) int64 {
	t.Helper()
	wh, err := svc.Create(t.Context(), FeishuCreateRequest{
		Name:       "dead-letter-target",
		WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/deadletter",
	}, "tester")
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}
	return wh.ID
}

// TestDeadLetterIsUniquePerPayload pins the constraint the retry counter
// depends on.
//
// A dead letter is one failed notification with a count of how many times
// sending it was tried. Two rows for the same payload mean the operator sees
// the same failure twice and each copy retries on its own schedule — the
// recipient gets the message twice if it later succeeds.
//
// The check writes through ent rather than through RecordDeadLetter, because
// the point is that the database refuses the duplicate. A service-level check
// can only show that this particular caller happens not to make one.
func TestDeadLetterIsUniquePerPayload(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	webhookID := seedDeadLetterWebhook(t, svc)

	const payload = `{"msg_type":"text","content":{"text":"hi"}}`

	if err := svc.RecordDeadLetter(t.Context(), webhookID, payload, "connection refused"); err != nil {
		t.Fatalf("first record: %v", err)
	}

	err := svc.client.FeishuDeadLetter.Create().
		SetWebhookID(webhookID).
		SetPayload(payload).
		SetPayloadHash(hashPayload(payload)).
		SetErrorMessage("connection refused").
		SetAttemptCount(1).
		Exec(t.Context())
	if err == nil {
		t.Error("a second row for the same payload was accepted — the unique index is missing")
	}
}

// TestDeadLetterConcurrentRecordsCollapse checks the counter survives overlap.
//
// Retries run concurrently across workers, so the same notification fails in
// several of them at once. Every attempt has to land on the one row: a lost
// increment delays the point at which the dead letter is given up on and
// cleaned away.
func TestDeadLetterConcurrentRecordsCollapse(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	webhookID := seedDeadLetterWebhook(t, svc)

	const payload = `{"msg_type":"text","content":{"text":"concurrent"}}`
	const attempts = 8

	var start, ready, done sync.WaitGroup
	start.Add(1)
	ready.Add(attempts)
	done.Add(attempts)

	errs := make([]error, attempts)
	for i := range attempts {
		go func() {
			defer done.Done()
			ready.Done()
			start.Wait()
			errs[i] = svc.RecordDeadLetter(t.Context(), webhookID, payload, "timeout")
		}()
	}
	ready.Wait()
	start.Done()
	done.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("attempt %d: %v", i, err)
		}
	}

	items, err := svc.ListDeadLetters(t.Context(), webhookID, 10)
	if err != nil {
		t.Fatalf("ListDeadLetters: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("dead letters = %d, want 1", len(items))
	}
	if items[0].AttemptCount != attempts {
		t.Errorf("attempt_count = %d, want %d — increments were lost",
			items[0].AttemptCount, attempts)
	}
}

// TestDeadLetterDistinctPayloadsAreSeparate checks the key is the payload, not
// the webhook.
//
// Deduping on webhook alone would fold every failure for one destination into a
// single row, and the operator would see one failure where there were three.
func TestDeadLetterDistinctPayloadsAreSeparate(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	webhookID := seedDeadLetterWebhook(t, svc)

	for _, p := range []string{`{"a":1}`, `{"a":2}`, `{"a":3}`} {
		if err := svc.RecordDeadLetter(t.Context(), webhookID, p, "timeout"); err != nil {
			t.Fatalf("record %s: %v", p, err)
		}
	}

	items, err := svc.ListDeadLetters(t.Context(), webhookID, 10)
	if err != nil {
		t.Fatalf("ListDeadLetters: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("dead letters = %d, want 3", len(items))
	}
}

// incompressiblePayload builds a payload that a btree index cannot fit.
//
// Repetitive filler will not do: PostgreSQL compresses the index entry, so
// strings.Repeat("x", 100000) indexes fine and a test built on it passes
// against the very design it is meant to reject. Chained hashes have no
// structure to compress.
//
// Deterministic rather than random, so a failure reproduces.
func incompressiblePayload(t *testing.T, minBytes int) string {
	t.Helper()
	var b strings.Builder
	sum := sha256.Sum256([]byte("dead-letter-size-probe"))
	for b.Len() < minBytes {
		b.WriteString(hex.EncodeToString(sum[:]))
		sum = sha256.Sum256(sum[:])
	}
	return `{"msg_type":"text","content":{"text":"` + b.String() + `"}}`
}

// TestDeadLetterAcceptsOversizePayload is the reason the index is on a hash.
//
// PostgreSQL caps a btree entry at 2704 bytes, so a unique index over the
// payload column itself rejects any notification larger than that — and a card
// carrying a long SQL statement is. The rejection would land on the error path,
// where it is least likely to be noticed.
//
// 6 KiB is chosen to clear the cap with room to spare; the probe that
// established the number failed at 6424 bytes.
func TestDeadLetterAcceptsOversizePayload(t *testing.T) {
	svc := newTestFeishuWebhookService(t)
	webhookID := seedDeadLetterWebhook(t, svc)

	big := incompressiblePayload(t, 6*1024)

	if err := svc.RecordDeadLetter(t.Context(), webhookID, big, "timeout"); err != nil {
		t.Fatalf("record oversize payload: %v", err)
	}
	if err := svc.RecordDeadLetter(t.Context(), webhookID, big, "timeout"); err != nil {
		t.Fatalf("re-record oversize payload: %v", err)
	}

	items, err := svc.ListDeadLetters(t.Context(), webhookID, 10)
	if err != nil {
		t.Fatalf("ListDeadLetters: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("dead letters = %d, want 1", len(items))
	}
	if items[0].AttemptCount != 2 {
		t.Errorf("attempt_count = %d, want 2", items[0].AttemptCount)
	}
}
