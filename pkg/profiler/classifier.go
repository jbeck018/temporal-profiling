package profiler

import (
	"strings"
)

// Classifier maps gRPC method names to EventTypes.
type Classifier struct {
	methodMap map[string]EventType
}

// NewClassifier creates a new event classifier.
func NewClassifier() *Classifier {
	c := &Classifier{
		methodMap: make(map[string]EventType),
	}
	c.initMethodMap()
	return c
}

// initMethodMap initializes the mapping from gRPC methods to event types.
func (c *Classifier) initMethodMap() {
	// Temporal WorkflowService methods
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/StartWorkflowExecution"] = EventWorkflowStarted
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/SignalWorkflowExecution"] = EventSignalSent
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/SignalWithStartWorkflowExecution"] = EventSignalSent
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/QueryWorkflow"] = EventQueryHandled
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/TerminateWorkflowExecution"] = EventWorkflowTerminated
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RequestCancelWorkflowExecution"] = EventWorkflowCanceled

	// Workflow task responses
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RespondWorkflowTaskCompleted"] = EventWorkflowTaskCompleted
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RespondWorkflowTaskFailed"] = EventWorkflowTaskFailed

	// Activity task responses
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RespondActivityTaskCompleted"] = EventActivityCompleted
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RespondActivityTaskFailed"] = EventActivityFailed
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RespondActivityTaskCanceled"] = EventActivityCanceled
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RespondActivityTaskCompletedById"] = EventActivityCompleted
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RespondActivityTaskFailedById"] = EventActivityFailed
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RespondActivityTaskCanceledById"] = EventActivityCanceled

	// Polling methods (these are special - they indicate task availability)
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/PollWorkflowTaskQueue"] = EventWorkflowTaskStarted
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/PollActivityTaskQueue"] = EventActivityTaskStarted

	// Heartbeat
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RecordActivityTaskHeartbeat"] = EventActivityStarted
	c.methodMap["/temporal.api.workflowservice.v1.WorkflowService/RecordActivityTaskHeartbeatById"] = EventActivityStarted
}

// Classify returns the EventType for a gRPC method name.
func (c *Classifier) Classify(method string) EventType {
	if eventType, ok := c.methodMap[method]; ok {
		return eventType
	}
	return EventUnknown
}

// IsPollingMethod returns true if the method is a long-polling method.
func (c *Classifier) IsPollingMethod(method string) bool {
	return strings.Contains(method, "Poll") && strings.Contains(method, "TaskQueue")
}

// IsResponseMethod returns true if the method is a task response.
func (c *Classifier) IsResponseMethod(method string) bool {
	return strings.Contains(method, "Respond")
}

// IsHeartbeatMethod returns true if the method is a heartbeat.
func (c *Classifier) IsHeartbeatMethod(method string) bool {
	return strings.Contains(method, "Heartbeat")
}

// ShouldProfile returns true if the method should be profiled.
// Some methods like GetSystemInfo or health checks may be excluded.
func (c *Classifier) ShouldProfile(method string) bool {
	// Exclude system/metadata methods
	excludePatterns := []string{
		"GetSystemInfo",
		"GetClusterInfo",
		"GetSearchAttributes",
		"ListNamespaces",
		"DescribeNamespace",
		"RegisterNamespace",
		"UpdateNamespace",
	}

	for _, pattern := range excludePatterns {
		if strings.Contains(method, pattern) {
			return false
		}
	}

	return true
}

// ExtractServiceName extracts the service name from a full method path.
func ExtractServiceName(fullMethod string) string {
	// /temporal.api.workflowservice.v1.WorkflowService/StartWorkflowExecution
	// -> WorkflowService
	parts := strings.Split(fullMethod, "/")
	if len(parts) >= 2 {
		serviceParts := strings.Split(parts[1], ".")
		if len(serviceParts) > 0 {
			return serviceParts[len(serviceParts)-1]
		}
	}
	return ""
}

// ExtractMethodName extracts just the method name from a full method path.
func ExtractMethodName(fullMethod string) string {
	// /temporal.api.workflowservice.v1.WorkflowService/StartWorkflowExecution
	// -> StartWorkflowExecution
	parts := strings.Split(fullMethod, "/")
	if len(parts) >= 3 {
		return parts[2]
	}
	return fullMethod
}
