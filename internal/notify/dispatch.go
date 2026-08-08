package notify

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
)

// transport is one way of delivering a notification.
//
// An adapter's whole job is rendering and sending. Escaping, retries and rate
// limiting are its private business — which is the fix for escaping having been
// per-transport luck: Feishu ran user content through escapeFeishuContent and
// DingTalk interpolated it raw, because the two renderers were written at
// different call sites months apart.
type transport interface {
	// name identifies the channel in logs and in the preference whitelist.
	name() string
	// enabled reports whether this deployment has configured the channel.
	enabled() bool
	// send delivers the notification. It is called on its own goroutine.
	send(n Notification) error
}

// dispatch is the one place a notification becomes messages.
//
// It owns deduplication, preference filtering and the goroutines — once, rather
// than fourteen inline `if enabled` blocks that each had to be remembered. The
// five events that reached only DingTalk did so because a block was not copied;
// there is now no block to copy.
func (s *Service) dispatch(ctx context.Context, n Notification) {
	// Ticket events are deduplicated; the rest are not. A test message the
	// operator pressed twice should arrive twice, and an SLA reminder is
	// deduplicated by its own scheduler rather than by ticket identity.
	if n.TicketID != 0 {
		if !s.shouldNotify(ctx, n.TicketID, string(n.Event)) {
			return
		}
	}

	sent := s.fanOut(n)

	// Record only when something actually went out.
	//
	// The dedup row used to be written unconditionally after the send closure,
	// even when no channel was enabled and nothing was sent — so a deployment
	// that enabled a channel later never received the events it had already
	// "notified" about.
	if n.TicketID != 0 && sent > 0 {
		s.recordNotification(ctx, n.TicketID, string(n.Event))
	}
}

// fanOut delivers to every enabled transport the audience wants, and reports
// how many accepted the notification.
func (s *Service) fanOut(n Notification) int {
	var wg sync.WaitGroup
	var mu sync.Mutex
	sent := 0

	for _, t := range s.transports() {
		if !t.enabled() {
			continue
		}

		wg.Add(1)
		go func(t transport) {
			defer wg.Done()
			if err := t.send(n); err != nil {
				log.Printf("notify: %s: %s: %v", t.name(), n.Event, err)
				return
			}
			mu.Lock()
			sent++
			mu.Unlock()
		}(t)
	}

	// Waiting is what makes the dedup record honest, and it is what let the
	// tests drop their 90-line request recorder and its waitFor loop: a
	// fire-and-forget send is only observable by racing it.
	wg.Wait()
	return sent
}

// renderMarkdown lays a notification out as DingTalk markdown.
func renderMarkdown(n Notification) string {
	var b strings.Builder
	b.WriteString(escapeMarkdownContent(n.Summary))
	for _, f := range n.Fields {
		fmt.Fprintf(&b, "\n- **%s**: %s", f.Label, escapeMarkdownContent(f.Value))
	}
	return b.String()
}

// escapeMarkdownContent neutralizes user-supplied text in a markdown body.
//
// The DingTalk path had no equivalent of escapeFeishuContent and interpolated
// SQL summaries and review comments directly, so a comment containing markdown
// control characters rewrote the message around it. Backticks are the sharpest
// of those because the surrounding template uses them for emphasis.
func escapeMarkdownContent(v string) string {
	r := strings.NewReplacer(
		"`", "'",
		"\n", " ",
		"\r", " ",
	)
	return r.Replace(v)
}
