package delivery

import (
	"strings"
	"testing"
)

func TestBuildEmailMessage(t *testing.T) {
	t.Parallel()

	msg := buildEmailMessage("from@example.test", EmailRequest{To: "to@example.test", Subject: "hello", Body: "line1\nline2", AttemptID: "attempt-1", NotificationID: "notif-1"})
	for _, want := range []string{"To: to@example.test", "From: from@example.test", "Subject: hello", "X-Notification-Attempt-ID: attempt-1", "X-Notification-ID: notif-1", "Message-ID: <attempt-1@notification-service>", "line1\r\nline2"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message missing %q in %q", want, msg)
		}
	}
}
