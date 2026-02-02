// Package profiler provides the core profiling engine and event types.
package profiler

import (
	"time"
)

// EventType classifies the type of Temporal operation being profiled.
type EventType int

const (
	// Workflow events
	EventWorkflowStarted EventType = iota
	EventWorkflowCompleted
	EventWorkflowFailed
	EventWorkflowCanceled
	EventWorkflowTerminated
	EventWorkflowTimedOut
	EventWorkflowContinuedAsNew

	// Activity events
	EventActivityScheduled
	EventActivityStarted
	EventActivityCompleted
	EventActivityFailed
	EventActivityTimedOut
	EventActivityCanceled

	// Task events
	EventWorkflowTaskStarted
	EventWorkflowTaskCompleted
	EventWorkflowTaskFailed
	EventActivityTaskStarted
	EventActivityTaskCompleted

	// Signal and query events
	EventSignalReceived
	EventSignalSent
	EventQueryHandled

	// Timer events
	EventTimerStarted
	EventTimerFired
	EventTimerCanceled

	// Child workflow events
	EventChildWorkflowStarted
	EventChildWorkflowCompleted
	EventChildWorkflowFailed

	// Other events
	EventUnknown
)

// String returns the string representation of an EventType.
func (e EventType) String() string {
	names := map[EventType]string{
		EventWorkflowStarted:        "workflow_started",
		EventWorkflowCompleted:      "workflow_completed",
		EventWorkflowFailed:         "workflow_failed",
		EventWorkflowCanceled:       "workflow_canceled",
		EventWorkflowTerminated:     "workflow_terminated",
		EventWorkflowTimedOut:       "workflow_timed_out",
		EventWorkflowContinuedAsNew: "workflow_continued_as_new",
		EventActivityScheduled:      "activity_scheduled",
		EventActivityStarted:        "activity_started",
		EventActivityCompleted:      "activity_completed",
		EventActivityFailed:         "activity_failed",
		EventActivityTimedOut:       "activity_timed_out",
		EventActivityCanceled:       "activity_canceled",
		EventWorkflowTaskStarted:    "workflow_task_started",
		EventWorkflowTaskCompleted:  "workflow_task_completed",
		EventWorkflowTaskFailed:     "workflow_task_failed",
		EventActivityTaskStarted:    "activity_task_started",
		EventActivityTaskCompleted:  "activity_task_completed",
		EventSignalReceived:         "signal_received",
		EventSignalSent:             "signal_sent",
		EventQueryHandled:           "query_handled",
		EventTimerStarted:           "timer_started",
		EventTimerFired:             "timer_fired",
		EventTimerCanceled:          "timer_canceled",
		EventChildWorkflowStarted:   "child_workflow_started",
		EventChildWorkflowCompleted: "child_workflow_completed",
		EventChildWorkflowFailed:    "child_workflow_failed",
		EventUnknown:                "unknown",
	}
	if name, ok := names[e]; ok {
		return name
	}
	return "unknown"
}

// Status represents the outcome status of an operation.
type Status int

const (
	StatusOK Status = iota
	StatusError
	StatusTimeout
	StatusCanceled
)

// String returns the string representation of a Status.
func (s Status) String() string {
	switch s {
	case StatusOK:
		return "ok"
	case StatusError:
		return "error"
	case StatusTimeout:
		return "timeout"
	case StatusCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// ProfileEvent represents a single profiling event captured from Temporal operations.
type ProfileEvent struct {
	// Identifiers
	WorkflowID   string `json:"workflow_id,omitempty"`
	RunID        string `json:"run_id,omitempty"`
	WorkflowType string `json:"workflow_type,omitempty"`
	ActivityID   string `json:"activity_id,omitempty"`
	ActivityType string `json:"activity_type,omitempty"`
	TaskQueue    string `json:"task_queue,omitempty"`
	Namespace    string `json:"namespace,omitempty"`

	// Timing
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration_ns"`

	// Classification
	EventType     EventType `json:"event_type"`
	OperationType string    `json:"operation_type"`

	// Status
	Status       Status `json:"status"`
	ErrorMessage string `json:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty"`

	// Metadata
	Attributes map[string]string `json:"attributes,omitempty"`

	// Tracing context (for OTEL correlation)
	TraceID      string `json:"trace_id,omitempty"`
	SpanID       string `json:"span_id,omitempty"`
	ParentSpanID string `json:"parent_span_id,omitempty"`

	// Worker information
	WorkerID   string `json:"worker_id,omitempty"`
	WorkerHost string `json:"worker_host,omitempty"`

	// Additional context
	Attempt                 int32         `json:"attempt,omitempty"`
	MaxAttempts             int32         `json:"max_attempts,omitempty"`
	ScheduleToStartDuration time.Duration `json:"schedule_to_start_duration_ns,omitempty"`
	StartToCloseDuration    time.Duration `json:"start_to_close_duration_ns,omitempty"`
}

// Clone creates a deep copy of the ProfileEvent.
func (e *ProfileEvent) Clone() *ProfileEvent {
	clone := *e
	if e.Attributes != nil {
		clone.Attributes = make(map[string]string, len(e.Attributes))
		for k, v := range e.Attributes {
			clone.Attributes[k] = v
		}
	}
	return &clone
}

// IsError returns true if the event represents an error condition.
func (e *ProfileEvent) IsError() bool {
	return e.Status == StatusError || e.Status == StatusTimeout
}

// IsWorkflowEvent returns true if this is a workflow-level event.
func (e *ProfileEvent) IsWorkflowEvent() bool {
	switch e.EventType {
	case EventWorkflowStarted, EventWorkflowCompleted, EventWorkflowFailed,
		EventWorkflowCanceled, EventWorkflowTerminated, EventWorkflowTimedOut,
		EventWorkflowContinuedAsNew:
		return true
	}
	return false
}

// IsActivityEvent returns true if this is an activity-level event.
func (e *ProfileEvent) IsActivityEvent() bool {
	switch e.EventType {
	case EventActivityScheduled, EventActivityStarted, EventActivityCompleted,
		EventActivityFailed, EventActivityTimedOut, EventActivityCanceled:
		return true
	}
	return false
}
