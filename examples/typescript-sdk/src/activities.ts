/**
 * Example activity
 */
export async function greet(name: string): Promise<string> {
  // Simulate some work
  await new Promise((resolve) => setTimeout(resolve, 100));
  return `Hello, ${name}!`;
}
