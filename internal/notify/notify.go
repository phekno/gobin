// Package notify dispatches webhook notifications on download events.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"text/template"
	"time"

	"github.com/phekno/gobin/internal/config"
)

// Event represents a download lifecycle event.
type Event struct {
	Type     string // "complete", "failed"
	Name     string
	Category string
	Status   string
	Size     int64
	Duration time.Duration
	Error    string
}

// Notifier dispatches webhook notifications.
type Notifier struct {
	cfgMgr *config.Manager
	client *http.Client
}

// New creates a notifier.
func New(cfgMgr *config.Manager) *Notifier {
	return &Notifier{
		cfgMgr: cfgMgr,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// Notify sends notifications for the given event based on config.
func (n *Notifier) Notify(ctx context.Context, event Event) {
	cfg := n.cfgMgr.Get()
	notifyCfg := cfg.Notifications

	// Check if this event type should trigger notifications
	switch event.Type {
	case "complete":
		if !notifyCfg.OnComplete {
			return
		}
	case "failed":
		if !notifyCfg.OnFailure {
			return
		}
	default:
		return
	}

	for _, webhook := range notifyCfg.Webhooks {
		if err := n.sendWebhook(ctx, webhook, event); err != nil {
			slog.Error("webhook notification failed",
				"webhook", webhook.Name,
				"error", err,
			)
		} else {
			slog.Info("webhook notification sent",
				"webhook", webhook.Name,
				"event", event.Type,
				"name", event.Name,
			)
		}
	}
}

func (n *Notifier) sendWebhook(ctx context.Context, webhook config.Webhook, event Event) error {
	var body []byte

	if webhook.Template != "" {
		// Use Go template to render the body
		tmpl, err := template.New("webhook").Parse(webhook.Template)
		if err != nil {
			return fmt.Errorf("parsing template: %w", err)
		}

		data := map[string]any{
			"Name":     event.Name,
			"Category": event.Category,
			"Status":   event.Status,
			"Type":     event.Type,
			"Size":     event.Size,
			"Duration": event.Duration.Round(time.Second).String(),
			"Error":    event.Error,
		}

		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return fmt.Errorf("executing template: %w", err)
		}
		body = buf.Bytes()
	} else {
		// Default JSON payload
		payload := map[string]any{
			"event":    event.Type,
			"name":     event.Name,
			"category": event.Category,
			"status":   event.Status,
			"size":     event.Size,
			"duration": event.Duration.Round(time.Second).String(),
		}
		if event.Error != "" {
			payload["error"] = event.Error
		}
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("marshalling payload: %w", err)
		}
	}

	// Detect content type
	contentType := "application/json"
	trimmed := strings.TrimSpace(string(body))
	if len(trimmed) > 0 && trimmed[0] != '{' && trimmed[0] != '[' {
		contentType = "text/plain"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhook.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "GoBin/1.0")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned %d", resp.StatusCode)
	}

	return nil
}
