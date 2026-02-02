// Example Go SDK integration with Temporal Profiler
//
// To use the profiler, simply point your Temporal client to the profiler's address
// instead of the Temporal server directly.
//
// Before: HostPort: "localhost:7233" (direct to Temporal)
// After:  HostPort: "localhost:7234" (through profiler)

package main

import (
	"context"
	"log"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

const (
	// Point to the profiler proxy instead of Temporal directly
	// The profiler will forward all traffic to the actual Temporal server
	TemporalHostPort = "localhost:7234" // Profiler listens here
	// TemporalHostPort = "localhost:7233" // Direct to Temporal (without profiler)

	TaskQueue = "example-task-queue"
)

func main() {
	// Create Temporal client pointing to the profiler
	c, err := client.Dial(client.Options{
		HostPort:  TemporalHostPort,
		Namespace: "default",
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer c.Close()

	// Create worker
	w := worker.New(c, TaskQueue, worker.Options{})
	w.RegisterWorkflow(ExampleWorkflow)
	w.RegisterActivity(ExampleActivity)

	// Start worker in background
	go func() {
		if err := w.Run(worker.InterruptCh()); err != nil {
			log.Fatalf("Failed to start worker: %v", err)
		}
	}()

	// Execute workflow
	workflowOptions := client.StartWorkflowOptions{
		ID:        "example-workflow-" + time.Now().Format("20060102150405"),
		TaskQueue: TaskQueue,
	}

	we, err := c.ExecuteWorkflow(context.Background(), workflowOptions, ExampleWorkflow, "World")
	if err != nil {
		log.Fatalf("Failed to execute workflow: %v", err)
	}

	log.Printf("Started workflow: WorkflowID=%s, RunID=%s", we.GetID(), we.GetRunID())

	// Wait for result
	var result string
	if err := we.Get(context.Background(), &result); err != nil {
		log.Fatalf("Workflow failed: %v", err)
	}

	log.Printf("Workflow completed: %s", result)
}

// ExampleWorkflow is a simple workflow that calls an activity
func ExampleWorkflow(ctx workflow.Context, name string) (string, error) {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	var result string
	err := workflow.ExecuteActivity(ctx, ExampleActivity, name).Get(ctx, &result)
	if err != nil {
		return "", err
	}

	return result, nil
}

// ExampleActivity is a simple activity
func ExampleActivity(ctx context.Context, name string) (string, error) {
	// Simulate some work
	time.Sleep(100 * time.Millisecond)
	return "Hello, " + name + "!", nil
}
