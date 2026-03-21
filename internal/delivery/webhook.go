package delivery

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type WebhookRequest struct {
	URL            string
	Body           string
	AttemptID      string
	NotificationID string
}

type WebhookSender struct {
	client *http.Client
}

func NewWebhookSender(timeout time.Duration) *WebhookSender {
	return &WebhookSender{client: &http.Client{Timeout: timeout}}
}

func (s *WebhookSender) Send(ctx context.Context, req WebhookRequest) (string, error) {
	ctx, span := otel.Tracer("notification-platform/delivery").Start(ctx, "webhook.send")
	defer span.End()
	span.SetAttributes(attribute.String("delivery.channel", "webhook"), attribute.String("http.url", req.URL))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, req.URL, strings.NewReader(req.Body))
	if err != nil {
		return "", fmt.Errorf("build webhook request: %w", err)
	}
	httpReq.Header.Set("Content-Type", contentTypeForBody(req.Body))
	if id := sanitizeHeaderValue(req.AttemptID); id != "" {
		httpReq.Header.Set("Idempotency-Key", id)
		httpReq.Header.Set("X-Notification-Attempt-ID", id)
	}
	if id := sanitizeHeaderValue(req.NotificationID); id != "" {
		httpReq.Header.Set("X-Notification-ID", id)
	}

	resp, err := s.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("send webhook request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	bodySnippet := strings.TrimSpace(string(bodyBytes))
	providerID := strings.TrimSpace(resp.Header.Get("X-Request-Id"))
	if providerID == "" {
		providerID = strings.TrimSpace(resp.Header.Get("X-Correlation-Id"))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if bodySnippet != "" {
			return providerID, fmt.Errorf("webhook returned %d: %s", resp.StatusCode, bodySnippet)
		}
		return providerID, fmt.Errorf("webhook returned %d", resp.StatusCode)
	}
	return providerID, nil
}

func contentTypeForBody(body string) string {
	trimmed := strings.TrimSpace(body)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return "application/json"
	}
	return "text/plain; charset=utf-8"
}

// sanitizeHeaderValue strips CR and LF characters to prevent header injection.
func sanitizeHeaderValue(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return v
}
