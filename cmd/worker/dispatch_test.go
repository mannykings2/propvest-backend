package main

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/mannykings2/propvest-backend/internal/queue"
)

func TestFormatKobo(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{750000, "₦7,500.00"},
		{100, "₦1.00"},
		{0, "₦0.00"},
		{12345678901, "₦123,456,789.01"},
		{-250000, "-₦2,500.00"},
	}
	for _, c := range cases {
		if got := formatKobo(c.in); got != c.want {
			t.Errorf("formatKobo(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBusinessEventNotification(t *testing.T) {
	uid := "33f7bc1f-169b-4245-bd07-83e80375196f"

	cases := []struct {
		name    string
		payload map[string]interface{}
		wantOK  bool
		wantTyp string
		wantSub string
		wantIn  string // body substring
	}{
		{
			name: "investment confirmed addresses the investor",
			payload: map[string]interface{}{
				"event_type":     "investment.created",
				"user_id":        uid,
				"property_title": "Sunrise Courts",
				"amount_kobo":    float64(250000),
				"slots":          float64(2),
				"reference":      "INV-1",
			},
			wantOK: true, wantTyp: "investment_created", wantSub: "Investment confirmed",
			wantIn: "₦2,500.00 investment in Sunrise Courts (2 slot(s))",
		},
		{
			name: "property published addresses the publisher",
			payload: map[string]interface{}{
				"event_type": "property.published",
				"admin_id":   uid,
				"title":      "Palm Grove",
			},
			wantOK: true, wantTyp: "property_update", wantSub: "Property published",
			wantIn: "Palm Grove is now live",
		},
		{
			name: "investment created without user_id has no recipient",
			payload: map[string]interface{}{
				"event_type": "investment.created",
			},
			wantOK: false,
		},
		{
			name: "event type with no rule is not notified",
			payload: map[string]interface{}{
				"event_type": "property.funded",
				"admin_id":   uid,
			},
			wantOK: false,
		},
		{
			name:    "unknown event type is not notified",
			payload: map[string]interface{}{"event_type": "something.new"},
			wantOK:  false,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			recipient, nType, title, body, ok := businessEventNotification(tt.payload)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if nType != tt.wantTyp {
				t.Errorf("type = %q, want %q", nType, tt.wantTyp)
			}
			if title != tt.wantSub {
				t.Errorf("title = %q, want %q", title, tt.wantSub)
			}
			if !strings.Contains(body, tt.wantIn) {
				t.Errorf("body = %q, want substring %q", body, tt.wantIn)
			}
			if recipient == uuid.Nil {
				t.Error("expected an addressable recipient")
			}
		})
	}
}

func TestRenderHTML(t *testing.T) {
	t.Run("keeps producer html", func(t *testing.T) {
		got := renderHTML(queue.EmailMessage{HTMLBody: "<p>rich</p>"})
		if got != "<p>rich</p>" {
			t.Errorf("got %q", got)
		}
	})

	t.Run("escapes text-only bodies", func(t *testing.T) {
		got := renderHTML(queue.EmailMessage{TextBody: "line1\n<b>x</b>"})
		if strings.Contains(got, "<b>x</b>") {
			t.Errorf("text body must be escaped, got %q", got)
		}
		if !strings.Contains(got, "line1<br>") {
			t.Errorf("newlines should become <br>, got %q", got)
		}
	})
}
