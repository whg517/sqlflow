package notify

import (
	"context"
)

// Channel names. These are what a preference row stores, so they are the same
// strings the settings UI offers.
const (
	ChannelDingTalk = "dingtalk"
	ChannelFeishu   = "feishu"
	ChannelWebhook  = "webhook"
)

// transports lists every adapter, which is the whole registry.
//
// Adding a fourth channel is one type and one line here. It used to be seven
// new `if s.isXEnabled()` blocks plus seven re-derivations of the field list,
// with five events silently left out unless someone noticed.
func (s *Service) transports() []transport {
	s.mu.RLock()
	stub := s.transportsForTest
	s.mu.RUnlock()
	if stub != nil {
		return stub
	}
	return []transport{
		&dingTalkTransport{svc: s},
		&feishuTransport{svc: s},
		&webhookTransport{svc: s},
	}
}

// --- DingTalk ---

type dingTalkTransport struct{ svc *Service }

func (d *dingTalkTransport) name() string { return ChannelDingTalk }
func (d *dingTalkTransport) enabled() bool {
	return d.svc.isEnabled()
}
func (d *dingTalkTransport) send(n Notification) error {
	d.svc.sendMarkdown(n.Title, renderMarkdown(n))
	return nil
}

// --- Feishu ---

type feishuTransport struct{ svc *Service }

func (f *feishuTransport) name() string { return ChannelFeishu }
func (f *feishuTransport) enabled() bool {
	return f.svc.isFeishuEnabled()
}
func (f *feishuTransport) send(n Notification) error {
	elements := make([]feishuCardElement, 0, len(n.Fields))
	for _, field := range n.Fields {
		elements = append(elements, feishuCardField(field.Label, field.Value))
	}
	f.svc.sendFeishuCard(n.Title, n.Summary, elements)
	return nil
}

// --- Outbound webhooks ---

// webhookTransport delivers to subscribed endpoints.
//
// This is the third transport that was already written and unreachable:
// DeliverEvent signs with HMAC, retries, and auto-disables a subscription after
// ten consecutive failures — and had zero production callers, because the
// subscription vocabulary ("ticket.approved") could not match the sender's
// ("approved"). One event identity is what connects it.
type webhookTransport struct{ svc *Service }

func (w *webhookTransport) name() string { return ChannelWebhook }

func (w *webhookTransport) enabled() bool {
	w.svc.mu.RLock()
	defer w.svc.mu.RUnlock()
	return w.svc.subSvc != nil
}

func (w *webhookTransport) send(n Notification) error {
	w.svc.mu.RLock()
	sub := w.svc.subSvc
	w.svc.mu.RUnlock()
	if sub == nil {
		return nil
	}

	payload := map[string]interface{}{
		"event":     string(n.Event),
		"title":     n.Title,
		"summary":   n.Summary,
		"ticket_id": n.TicketID,
	}
	fields := make(map[string]string, len(n.Fields))
	for _, f := range n.Fields {
		fields[f.Label] = f.Value
	}
	payload["fields"] = fields

	// DeliverEvent fans out to every subscription whose event list includes this
	// one, signs each request and handles its own retries; a per-endpoint
	// failure is its business, not the dispatcher's.
	sub.DeliverEvent(context.Background(), string(n.Event), payload)
	return nil
}
