// Package slack provides a Slack notification sink for profiling alerts.
package slack

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/temporal-profiling/temporal-profiler/pkg/config"
	"github.com/temporal-profiling/temporal-profiler/pkg/profiler"
	"github.com/temporal-profiling/temporal-profiler/pkg/sink"
)

// Severity represents alert severity levels.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarning
	SeverityError
	SeverityCritical
)

// String returns the string representation of severity.
func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "info"
	case SeverityWarning:
		return "warning"
	case SeverityError:
		return "error"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// Color returns the Slack attachment color for severity.
func (s Severity) Color() string {
	switch s {
	case SeverityInfo:
		return "#36a64f" // Green
	case SeverityWarning:
		return "#FFA500" // Orange
	case SeverityError:
		return "#FF0000" // Red
	case SeverityCritical:
		return "#8B0000" // Dark red
	default:
		return "#808080" // Gray
	}
}

// ParseSeverity parses a severity string.
func ParseSeverity(s string) Severity {
	switch strings.ToLower(s) {
	case "info":
		return SeverityInfo
	case "warning":
		return SeverityWarning
	case "error":
		return SeverityError
	case "critical":
		return SeverityCritical
	default:
		return SeverityWarning
	}
}

// AlertRule defines a rule for triggering alerts.
type AlertRule struct {
	Name      string
	Condition AlertCondition
	Severity  Severity
}

// AlertCondition is a function that checks if an event matches the alert condition.
type AlertCondition func(event *profiler.ProfileEvent) bool

// SlackMessage represents a Slack webhook message.
type SlackMessage struct {
	Channel     string       `json:"channel,omitempty"`
	Username    string       `json:"username,omitempty"`
	IconEmoji   string       `json:"icon_emoji,omitempty"`
	Text        string       `json:"text,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}

// Attachment represents a Slack message attachment.
type Attachment struct {
	Color      string   `json:"color,omitempty"`
	Title      string   `json:"title,omitempty"`
	TitleLink  string   `json:"title_link,omitempty"`
	Text       string   `json:"text,omitempty"`
	Fields     []Field  `json:"fields,omitempty"`
	Footer     string   `json:"footer,omitempty"`
	FooterIcon string   `json:"footer_icon,omitempty"`
	Timestamp  int64    `json:"ts,omitempty"`
	MarkdownIn []string `json:"mrkdwn_in,omitempty"`
}

// Field represents a Slack attachment field.
type Field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

// Sink sends alerts to Slack via webhooks.
type Sink struct {
	*sink.BaseSink

	config     *config.SlackSinkConfig
	logger     *zap.Logger
	httpClient *http.Client
	rules      []AlertRule
	cooldowns  map[string]time.Time
	mu         sync.RWMutex

	// Rate limiting
	lastAlert  time.Time
	alertCount int
}

// NewSink creates a new Slack sink.
func NewSink(cfg *config.SlackSinkConfig, logger *zap.Logger) *Sink {
	return &Sink{
		BaseSink:  sink.NewBaseSink("slack"),
		config:    cfg,
		logger:    logger,
		cooldowns: make(map[string]time.Time),
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Init initializes the Slack sink.
func (s *Sink) Init(ctx context.Context) error {
	// Parse alert rules from config
	s.rules = s.parseAlertRules()

	s.logger.Info("Slack sink initialized",
		zap.Int("rules", len(s.rules)),
		zap.String("channel", s.config.Channel),
	)

	return nil
}

// parseAlertRules parses alert rules from configuration.
func (s *Sink) parseAlertRules() []AlertRule {
	rules := make([]AlertRule, 0, len(s.config.Alerts))

	for _, cfg := range s.config.Alerts {
		rule := AlertRule{
			Name:     cfg.Name,
			Severity: ParseSeverity(cfg.Severity),
		}

		// Parse condition string into a function
		rule.Condition = s.parseCondition(cfg.Condition)
		rules = append(rules, rule)
	}

	// Add default rules if none configured
	if len(rules) == 0 {
		rules = append(rules,
			AlertRule{
				Name:     "workflow_failed",
				Severity: SeverityError,
				Condition: func(e *profiler.ProfileEvent) bool {
					return e.EventType == profiler.EventWorkflowFailed
				},
			},
			AlertRule{
				Name:     "activity_failed",
				Severity: SeverityWarning,
				Condition: func(e *profiler.ProfileEvent) bool {
					return e.EventType == profiler.EventActivityFailed
				},
			},
			AlertRule{
				Name:     "slow_workflow",
				Severity: SeverityWarning,
				Condition: func(e *profiler.ProfileEvent) bool {
					return e.IsWorkflowEvent() && e.Duration > 30*time.Second
				},
			},
		)
	}

	return rules
}

// parseCondition parses a condition string into a function.
func (s *Sink) parseCondition(condition string) AlertCondition {
	condition = strings.TrimSpace(condition)

	// Parse common condition patterns
	switch {
	case strings.Contains(condition, "workflow_duration >"):
		var threshold time.Duration
		fmt.Sscanf(condition, "workflow_duration > %v", &threshold)
		return func(e *profiler.ProfileEvent) bool {
			return e.IsWorkflowEvent() && e.Duration > threshold
		}

	case strings.Contains(condition, "activity_duration >"):
		var threshold time.Duration
		fmt.Sscanf(condition, "activity_duration > %v", &threshold)
		return func(e *profiler.ProfileEvent) bool {
			return e.IsActivityEvent() && e.Duration > threshold
		}

	case condition == "event_type == WORKFLOW_FAILED":
		return func(e *profiler.ProfileEvent) bool {
			return e.EventType == profiler.EventWorkflowFailed
		}

	case condition == "event_type == ACTIVITY_FAILED":
		return func(e *profiler.ProfileEvent) bool {
			return e.EventType == profiler.EventActivityFailed
		}

	case strings.Contains(condition, "status == error"):
		return func(e *profiler.ProfileEvent) bool {
			return e.Status == profiler.StatusError
		}

	default:
		// Default: match errors
		return func(e *profiler.ProfileEvent) bool {
			return e.IsError()
		}
	}
}

// Write writes events to Slack (sends alerts for matching events).
func (s *Sink) Write(ctx context.Context, events []*profiler.ProfileEvent) error {
	for _, event := range events {
		for _, rule := range s.rules {
			if rule.Condition(event) {
				if s.shouldAlert(rule.Name) {
					if err := s.sendAlert(ctx, event, rule); err != nil {
						s.logger.Error("failed to send Slack alert",
							zap.String("rule", rule.Name),
							zap.Error(err),
						)
					}
				}
			}
		}
	}
	return nil
}

// shouldAlert checks if we should send an alert (respecting rate limits and cooldowns).
func (s *Sink) shouldAlert(ruleName string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// Check global rate limit
	if s.config.RateLimit.MaxPerMinute > 0 {
		if now.Sub(s.lastAlert) < time.Minute {
			if s.alertCount >= s.config.RateLimit.MaxPerMinute {
				return false
			}
		} else {
			// Reset counter for new minute
			s.alertCount = 0
		}
	}

	// Check rule-specific cooldown
	if cooldownEnd, ok := s.cooldowns[ruleName]; ok {
		if now.Before(cooldownEnd) {
			return false
		}
	}

	// Update state
	s.lastAlert = now
	s.alertCount++
	if s.config.RateLimit.Cooldown > 0 {
		s.cooldowns[ruleName] = now.Add(s.config.RateLimit.Cooldown)
	}

	return true
}

// sendAlert sends a single alert to Slack.
func (s *Sink) sendAlert(ctx context.Context, event *profiler.ProfileEvent, rule AlertRule) error {
	msg := s.formatMessage(event, rule)

	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", s.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack returned status %d", resp.StatusCode)
	}

	s.logger.Debug("sent Slack alert",
		zap.String("rule", rule.Name),
		zap.String("workflow_id", event.WorkflowID),
	)

	return nil
}

// formatMessage formats a Slack message for an event.
func (s *Sink) formatMessage(event *profiler.ProfileEvent, rule AlertRule) *SlackMessage {
	fields := []Field{
		{Title: "Workflow ID", Value: event.WorkflowID, Short: true},
		{Title: "Workflow Type", Value: event.WorkflowType, Short: true},
		{Title: "Namespace", Value: event.Namespace, Short: true},
		{Title: "Task Queue", Value: event.TaskQueue, Short: true},
		{Title: "Duration", Value: event.Duration.String(), Short: true},
		{Title: "Status", Value: event.Status.String(), Short: true},
	}

	if event.ActivityType != "" {
		fields = append(fields,
			Field{Title: "Activity Type", Value: event.ActivityType, Short: true},
			Field{Title: "Activity ID", Value: event.ActivityID, Short: true},
		)
	}

	if event.ErrorMessage != "" {
		fields = append(fields, Field{
			Title: "Error",
			Value: truncateString(event.ErrorMessage, 200),
			Short: false,
		})
	}

	if event.Attempt > 1 {
		fields = append(fields, Field{
			Title: "Attempt",
			Value: fmt.Sprintf("%d", event.Attempt),
			Short: true,
		})
	}

	title := fmt.Sprintf("[%s] %s", strings.ToUpper(rule.Severity.String()), rule.Name)

	return &SlackMessage{
		Channel:   s.config.Channel,
		Username:  "Temporal Profiler",
		IconEmoji: ":chart_with_upwards_trend:",
		Attachments: []Attachment{
			{
				Color:      rule.Severity.Color(),
				Title:      title,
				Fields:     fields,
				Footer:     "Temporal Profiler",
				Timestamp:  event.Timestamp.Unix(),
				MarkdownIn: []string{"text", "fields"},
			},
		},
	}
}

// Flush flushes any pending data (no-op for Slack).
func (s *Sink) Flush(ctx context.Context) error {
	return nil
}

// Close closes the sink.
func (s *Sink) Close(ctx context.Context) error {
	return nil
}

// truncateString truncates a string to a maximum length.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
