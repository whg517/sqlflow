package notify

import (
	"context"
	"strings"
	"testing"

	"github.com/whg517/sqlflow/internal/model"
)

// recordingTransport is an adapter that keeps what it was asked to send.
//
// This is what the seam bought: a notification is a value, so the decision side
// is observable without an HTTP server. Testing it used to need a 90-line
// request recorder and a waitFor helper racing fire-and-forget goroutines,
// because a decision was only visible as an outbound POST.
type recordingTransport struct {
	channel string
	on      bool
	got     []Notification
}

func (r *recordingTransport) name() string  { return r.channel }
func (r *recordingTransport) enabled() bool { return r.on }
func (r *recordingTransport) send(n Notification) error {
	r.got = append(r.got, n)
	return nil
}

// TestEveryTicketEventReachesEveryEnabledTransport is the defect this whole
// change exists to remove.
//
// Five of twelve events reached DingTalk only — NotifyRiskAlert,
// SendTestMessage, NotifySLAReminderRaw, NotifySLAEscalateRaw and
// NotifySLAAutoReject — because each was written as a bare isEnabled +
// sendMarkdown rather than as a fan-out. Nothing declared that; it was an
// artifact of which `if` block someone remembered to copy. A Feishu-only
// deployment lost every SLA reminder and escalation, which are the events an
// approver most needs.
//
// There is no `if` block to copy now, and this asserts it for every event
// rather than for the ones someone thought to check.
func TestEveryTicketEventReachesEveryEnabledTransport(t *testing.T) {
	svc := NewService(Deps{})

	a := &recordingTransport{channel: "alpha", on: true}
	b := &recordingTransport{channel: "beta", on: true}
	svc.transportsForTest = []transport{a, b}

	ticket := &model.Ticket{
		ID: 1, SubmitterName: "dev", Database: "appdb",
		SQLSummary: "ALTER TABLE t ADD c INT", RiskLevel: "high",
	}
	ctx := context.Background()

	// Every entry point the platform has, including the three that used to be
	// DingTalk-only.
	emit := map[string]func(){
		"created":          func() { svc.NotifyTicketCreated(ctx, ticket) },
		"pending_approval": func() { svc.NotifyTicketPendingApproval(ctx, ticket) },
		"approved":         func() { svc.NotifyTicketApproved(ctx, ticket) },
		"rejected":         func() { svc.NotifyTicketRejected(ctx, ticket) },
		"scheduled":        func() { svc.NotifyTicketScheduled(ctx, ticket) },
		"executed":         func() { svc.NotifyTicketExecuted(ctx, ticket) },
		"failed":           func() { svc.NotifyTicketFailed(ctx, ticket, "boom") },
		"risk_alert":       func() { svc.NotifyRiskAlert("dev", "DROP TABLE t", "critical", 1, "appdb") },
		"test":             func() { svc.SendTestMessage() },
		"sla_reminder":     func() { svc.NotifySLAReminderRaw(1, "2", "4", "dba", 50) },
		"sla_escalate":     func() { svc.NotifySLAEscalateRaw(1, "4", "dba") },
		"sla_auto_reject":  func() { svc.NotifySLAAutoReject(1, "high", 60, "dev", "DROP TABLE t") },
	}

	for name, fn := range emit {
		t.Run(name, func(t *testing.T) {
			a.got, b.got = nil, nil
			fn()

			if len(a.got) != 1 {
				t.Fatalf("alpha received %d notifications, want 1", len(a.got))
			}
			if len(b.got) != 1 {
				t.Fatalf("beta received %d notifications, want 1 — a transport was skipped", len(b.got))
			}
			if a.got[0].Event != b.got[0].Event {
				t.Errorf("transports saw different events: %s vs %s", a.got[0].Event, b.got[0].Event)
			}
			if a.got[0].Title == "" || a.got[0].Summary == "" {
				t.Error("notification carries no title or summary")
			}
			if !IsKnownEvent(string(a.got[0].Event)) {
				t.Errorf("event %q is not in the declared set", a.got[0].Event)
			}
		})
	}
}

// TestDisabledTransportIsSkipped keeps the fan-out from becoming a broadcast to
// channels the operator never configured.
func TestDisabledTransportIsSkipped(t *testing.T) {
	svc := NewService(Deps{})
	on := &recordingTransport{channel: "on", on: true}
	off := &recordingTransport{channel: "off", on: false}
	svc.transportsForTest = []transport{on, off}

	svc.SendTestMessage()

	if len(on.got) != 1 {
		t.Errorf("enabled transport received %d, want 1", len(on.got))
	}
	if len(off.got) != 0 {
		t.Errorf("disabled transport received %d, want 0", len(off.got))
	}
}

// TestMarkdownEscapingIsNotPerTransport closes the gap where one renderer
// escaped user content and the other did not.
//
// Feishu ran values through escapeFeishuContent; DingTalk interpolated SQL
// summaries and review comments straight into a markdown body, because the two
// renderers were written at different call sites. Rendering belongs to the
// adapter now, so there is one place per transport to get this right instead of
// twelve.
func TestMarkdownEscapingIsNotPerTransport(t *testing.T) {
	n := Notification{
		Title:   "t",
		Summary: "s",
		Fields:  []Field{{"审批意见", "看起来`没问题`\n**已批准**"}},
	}

	got := renderMarkdown(n)
	if strings.Contains(got, "`") {
		t.Errorf("backticks survived into the markdown body: %q", got)
	}
	if strings.Contains(got, "看起来`") {
		t.Error("user content was interpolated unescaped")
	}
	if !strings.Contains(got, "审批意见") {
		t.Error("the field label was lost")
	}
}
