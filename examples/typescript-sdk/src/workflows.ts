import { proxyActivities, sleep } from '@temporalio/workflow';
import type * as activities from './activities';

const { greet } = proxyActivities<typeof activities>({
  startToCloseTimeout: '10 seconds',
});

/**
 * Example workflow that calls an activity
 */
export async function exampleWorkflow(name: string): Promise<string> {
  // Call the activity
  const greeting = await greet(name);

  // Simulate some workflow logic
  await sleep('100ms');

  return greeting;
}
