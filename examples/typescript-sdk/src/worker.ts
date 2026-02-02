import { Worker, NativeConnection } from '@temporalio/worker';
import * as activities from './activities';

// Point to the profiler proxy instead of Temporal directly
const TEMPORAL_ADDRESS = 'localhost:7234'; // Profiler listens here
// const TEMPORAL_ADDRESS = 'localhost:7233'; // Direct to Temporal (without profiler)

async function run() {
  // Connect to Temporal through the profiler
  const connection = await NativeConnection.connect({
    address: TEMPORAL_ADDRESS,
  });

  const worker = await Worker.create({
    connection,
    namespace: 'default',
    taskQueue: 'example-task-queue',
    workflowsPath: require.resolve('./workflows'),
    activities,
  });

  console.log('Worker started');
  await worker.run();
}

run().catch((err) => {
  console.error(err);
  process.exit(1);
});
