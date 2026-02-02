"""
Example Python SDK integration with Temporal Profiler

To use the profiler, simply point your Temporal client to the profiler's address
instead of the Temporal server directly.

Before: target_host="localhost:7233" (direct to Temporal)
After:  target_host="localhost:7234" (through profiler)
"""

import asyncio
from datetime import timedelta
from temporalio import activity, workflow
from temporalio.client import Client
from temporalio.worker import Worker

# Point to the profiler proxy instead of Temporal directly
TEMPORAL_ADDRESS = "localhost:7234"  # Profiler listens here
# TEMPORAL_ADDRESS = "localhost:7233"  # Direct to Temporal (without profiler)

TASK_QUEUE = "example-task-queue"


@activity.defn
async def greet(name: str) -> str:
    """Example activity that greets someone."""
    # Simulate some work
    await asyncio.sleep(0.1)
    return f"Hello, {name}!"


@workflow.defn
class ExampleWorkflow:
    """Example workflow that calls an activity."""

    @workflow.run
    async def run(self, name: str) -> str:
        # Call the activity
        result = await workflow.execute_activity(
            greet,
            name,
            start_to_close_timeout=timedelta(seconds=10),
        )
        return result


async def main():
    # Connect to Temporal through the profiler
    client = await Client.connect(TEMPORAL_ADDRESS, namespace="default")

    # Start worker in background
    async with Worker(
        client,
        task_queue=TASK_QUEUE,
        workflows=[ExampleWorkflow],
        activities=[greet],
    ):
        # Execute workflow
        result = await client.execute_workflow(
            ExampleWorkflow.run,
            "World",
            id=f"example-workflow-python",
            task_queue=TASK_QUEUE,
        )
        print(f"Workflow completed: {result}")


if __name__ == "__main__":
    asyncio.run(main())
