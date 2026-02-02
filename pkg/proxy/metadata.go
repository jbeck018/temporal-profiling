package proxy

import (
	"context"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/proto"

	workflowservice "go.temporal.io/api/workflowservice/v1"
)

// RequestMetadata holds metadata extracted from Temporal gRPC requests.
type RequestMetadata struct {
	WorkflowID   string
	RunID        string
	WorkflowType string
	ActivityID   string
	ActivityType string
	TaskQueue    string
	Namespace    string
	TaskToken    []byte
	Attempt      int32
}

// ExtractMetadata extracts Temporal metadata from a gRPC request.
func ExtractMetadata(ctx context.Context, req interface{}, method string) *RequestMetadata {
	meta := &RequestMetadata{}

	// Extract namespace from gRPC metadata if available
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ns := md.Get("temporal-namespace"); len(ns) > 0 {
			meta.Namespace = ns[0]
		}
	}

	// Extract from typed request
	switch r := req.(type) {
	case *workflowservice.StartWorkflowExecutionRequest:
		meta.WorkflowID = r.WorkflowId
		meta.Namespace = r.Namespace
		meta.TaskQueue = r.TaskQueue.GetName()
		meta.WorkflowType = r.WorkflowType.GetName()

	case *workflowservice.SignalWorkflowExecutionRequest:
		meta.WorkflowID = r.WorkflowExecution.GetWorkflowId()
		meta.RunID = r.WorkflowExecution.GetRunId()
		meta.Namespace = r.Namespace

	case *workflowservice.SignalWithStartWorkflowExecutionRequest:
		meta.WorkflowID = r.WorkflowId
		meta.Namespace = r.Namespace
		meta.TaskQueue = r.TaskQueue.GetName()
		meta.WorkflowType = r.WorkflowType.GetName()

	case *workflowservice.QueryWorkflowRequest:
		meta.WorkflowID = r.Execution.GetWorkflowId()
		meta.RunID = r.Execution.GetRunId()
		meta.Namespace = r.Namespace

	case *workflowservice.TerminateWorkflowExecutionRequest:
		meta.WorkflowID = r.WorkflowExecution.GetWorkflowId()
		meta.RunID = r.WorkflowExecution.GetRunId()
		meta.Namespace = r.Namespace

	case *workflowservice.RequestCancelWorkflowExecutionRequest:
		meta.WorkflowID = r.WorkflowExecution.GetWorkflowId()
		meta.RunID = r.WorkflowExecution.GetRunId()
		meta.Namespace = r.Namespace

	case *workflowservice.PollWorkflowTaskQueueRequest:
		meta.Namespace = r.Namespace
		meta.TaskQueue = r.TaskQueue.GetName()

	case *workflowservice.PollActivityTaskQueueRequest:
		meta.Namespace = r.Namespace
		meta.TaskQueue = r.TaskQueue.GetName()

	case *workflowservice.RespondWorkflowTaskCompletedRequest:
		meta.TaskToken = r.TaskToken
		meta.Namespace = r.Namespace

	case *workflowservice.RespondWorkflowTaskFailedRequest:
		meta.TaskToken = r.TaskToken
		meta.Namespace = r.Namespace

	case *workflowservice.RespondActivityTaskCompletedRequest:
		meta.TaskToken = r.TaskToken
		meta.Namespace = r.Namespace

	case *workflowservice.RespondActivityTaskFailedRequest:
		meta.TaskToken = r.TaskToken
		meta.Namespace = r.Namespace

	case *workflowservice.RespondActivityTaskCanceledRequest:
		meta.TaskToken = r.TaskToken
		meta.Namespace = r.Namespace

	case *workflowservice.RecordActivityTaskHeartbeatRequest:
		meta.TaskToken = r.TaskToken
		meta.Namespace = r.Namespace

	case *workflowservice.GetWorkflowExecutionHistoryRequest:
		meta.WorkflowID = r.Execution.GetWorkflowId()
		meta.RunID = r.Execution.GetRunId()
		meta.Namespace = r.Namespace

	case *workflowservice.DescribeWorkflowExecutionRequest:
		meta.WorkflowID = r.Execution.GetWorkflowId()
		meta.RunID = r.Execution.GetRunId()
		meta.Namespace = r.Namespace

	default:
		// Try to extract via reflection for unknown types
		extractGenericMetadata(req, meta)
	}

	return meta
}

// extractGenericMetadata attempts to extract common fields via proto reflection.
func extractGenericMetadata(req interface{}, meta *RequestMetadata) {
	msg, ok := req.(proto.Message)
	if !ok {
		return
	}

	// Use proto reflection to find common fields
	reflection := msg.ProtoReflect()
	fields := reflection.Descriptor().Fields()

	for i := 0; i < fields.Len(); i++ {
		field := fields.Get(i)
		name := string(field.Name())

		if !reflection.Has(field) {
			continue
		}

		switch name {
		case "namespace":
			if meta.Namespace == "" {
				meta.Namespace = reflection.Get(field).String()
			}
		case "workflow_id":
			if meta.WorkflowID == "" {
				meta.WorkflowID = reflection.Get(field).String()
			}
		case "run_id":
			if meta.RunID == "" {
				meta.RunID = reflection.Get(field).String()
			}
		}
	}
}

// ResponseMetadata holds metadata extracted from Temporal gRPC responses.
type ResponseMetadata struct {
	RunID        string
	WorkflowID   string
	WorkflowType string
	ActivityID   string
	ActivityType string
	TaskToken    []byte
	Attempt      int32
	TaskQueue    string
}

// ExtractResponseMetadata extracts metadata from a Temporal gRPC response.
func ExtractResponseMetadata(resp interface{}, method string) *ResponseMetadata {
	meta := &ResponseMetadata{}

	switch r := resp.(type) {
	case *workflowservice.StartWorkflowExecutionResponse:
		meta.RunID = r.RunId

	case *workflowservice.SignalWithStartWorkflowExecutionResponse:
		meta.RunID = r.RunId

	case *workflowservice.PollWorkflowTaskQueueResponse:
		if r.WorkflowExecution != nil {
			meta.WorkflowID = r.WorkflowExecution.WorkflowId
			meta.RunID = r.WorkflowExecution.RunId
		}
		if r.WorkflowType != nil {
			meta.WorkflowType = r.WorkflowType.Name
		}
		meta.TaskToken = r.TaskToken
		meta.Attempt = r.Attempt

	case *workflowservice.PollActivityTaskQueueResponse:
		if r.WorkflowExecution != nil {
			meta.WorkflowID = r.WorkflowExecution.WorkflowId
			meta.RunID = r.WorkflowExecution.RunId
		}
		if r.WorkflowType != nil {
			meta.WorkflowType = r.WorkflowType.Name
		}
		if r.ActivityType != nil {
			meta.ActivityType = r.ActivityType.Name
		}
		meta.ActivityID = r.ActivityId
		meta.TaskToken = r.TaskToken
		meta.Attempt = r.Attempt

	case *workflowservice.RespondWorkflowTaskCompletedResponse:
		// Response typically doesn't have much metadata

	case *workflowservice.RespondActivityTaskCompletedResponse:
		// Response typically doesn't have much metadata
	}

	return meta
}
