/**
 * Example TypeScript SDK integration with Temporal Profiler
 *
 * To use the profiler, simply point your Temporal connection to the profiler's address
 * instead of the Temporal server directly.
 *
 * Before: address: 'localhost:7233' (direct to Temporal)
 * After:  address: 'localhost:7234' (through profiler)
 */

import { Client, Connection } from '@temporalio/client';
import { exampleWorkflow } from './workflows';

// Point to the profiler proxy instead of Temporal directly
const TEMPORAL_ADDRESS = 'localhost:7234'; // Profiler listens here
// const TEMPORAL_ADDRESS = 'localhost:7233'; // Direct to Temporal (without profiler)

async function run() {
  // Connect to Temporal through the profiler
  const connection = await Connection.connect({
    address: TEMPORAL_ADDRESS,
  });

  const client = new Client({
    connection,
    namespace: 'default',
  });

  // Start a workflow
  const handle = await client.workflow.start(exampleWorkflow, {
    taskQueue: 'example-task-queue',
    workflowId: `example-workflow-${Date.now()}`,
    args: ['World'],
  });

  console.log(`Started workflow: ${handle.workflowId}`);

  // Wait for result
  const result = await handle.result();
  console.log(`Workflow completed: ${result}`);
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
