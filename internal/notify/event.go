package notify

// The notification seam.
//
// There was none: each of twelve events was a hand-written fan-out that decided
// what happened, which channels were enabled, how to render for each of them,
// and how to send — inline, twice over, once per transport. The knowledge "which
// fields describe a rejected ticket" lived twelve times rather than once, and
// the consequences were all realized in the tree:
//
//   - Five of twelve events reached DingTalk only, because whoever added them
//     copied one `if` block and not the other. A Feishu-only deployment lost
//     every SLA reminder and escalation, and nothing declared that.
//   - Escaping was per-transport: the Feishu renderer ran user content through
//     escapeFeishuContent, the DingTalk one interpolated SQL summaries and
//     review comments straight into markdown.
//   - The event had three disjoint spellings — "approved" for dedup,
//     "ticket_approved" for preferences, "ticket.approved" for subscriptions —
//     so neither consumer could ever match the producer. That is why
//     PreferenceService.ShouldNotify and WebhookSubscriptionService.DeliverEvent
//     both had zero production callers: two finished features, unreachable,
//     blocked by a naming mismatch.
//
// So: one event identity, one transport-neutral value, one dispatcher that owns
// dedup and channel selection, and one adapter per transport.

// EventType identifies a lifecycle moment worth telling someone about.
//
// One spelling, shared by the sender, the user preferences and the outbound
// webhook subscriptions. The three vocabularies that existed before could not
// be reconciled after the fact — they had to become the same thing.
type EventType string

const (
	EventTicketCreated         EventType = "ticket.created"
	EventTicketPendingApproval EventType = "ticket.pending_approval"
	EventTicketApproved        EventType = "ticket.approved"
	EventTicketRejected        EventType = "ticket.rejected"
	EventTicketScheduled       EventType = "ticket.scheduled"
	EventTicketExecuted        EventType = "ticket.executed"
	EventTicketFailed          EventType = "ticket.failed"
	EventRiskAlert             EventType = "risk.alert"
	EventSLAWarning            EventType = "sla.warning"
	EventSLABreached           EventType = "sla.breached"
	EventSLAAutoRejected       EventType = "sla.auto_rejected"
	EventTest                  EventType = "test.message"
)

// eventLabels is the display name for each event, and the single list every
// other list is derived from.
//
// The preference whitelist and the webhook subscription whitelist used to be
// independent literals; deriving them is what keeps a fourth spelling from
// appearing the next time an event is added.
var eventLabels = map[EventType]string{
	EventTicketCreated:         "工单创建",
	EventTicketPendingApproval: "待审批",
	EventTicketApproved:        "审批通过",
	EventTicketRejected:        "审批驳回",
	EventTicketScheduled:       "定时执行已安排",
	EventTicketExecuted:        "执行完成",
	EventTicketFailed:          "执行失败",
	EventRiskAlert:             "高风险告警",
	EventSLAWarning:            "SLA 预警",
	EventSLABreached:           "SLA 违规",
	EventSLAAutoRejected:       "SLA 超时自动驳回",
	EventTest:                  "测试消息",
}

// EventTypes returns every event, for the whitelists and the settings UI.
func EventTypes() map[string]string {
	out := make(map[string]string, len(eventLabels))
	for k, v := range eventLabels {
		out[string(k)] = v
	}
	return out
}

// IsKnownEvent reports whether this is an event the platform emits.
func IsKnownEvent(name string) bool {
	_, ok := eventLabels[EventType(name)]
	return ok
}

// Field is one labeled value in a notification.
//
// A notification is a title, a summary and an ordered list of these — a shape
// every transport can render and none of them defines. Rendering is the
// adapter's business; deciding what to say is not.
type Field struct {
	Label string
	Value string
}

// Notification is what happened, described without reference to how it is sent.
//
// It is a plain value, which is the point: the decision side is testable
// without an HTTP server. Testing it used to require a 90-line request recorder
// and a waitFor helper racing fire-and-forget goroutines, because a decision was
// only observable as an outbound POST.
type Notification struct {
	Event EventType

	// TicketID keys deduplication. Zero for events that are not about a ticket,
	// which are not deduplicated — a test message the operator pressed twice
	// should arrive twice.
	TicketID int64

	Title   string
	Summary string
	Fields  []Field
}

// Label is the human name of this notification's event.
func (n Notification) Label() string { return eventLabels[n.Event] }
