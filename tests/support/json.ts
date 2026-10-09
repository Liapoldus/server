// Fixture JSON is a test process boundary. Keep JSON.parse's untyped return
// isolated here; callers declare the shape they assert against.
export function parseJSON<T = unknown>(text: string): T {
  const value: unknown = JSON.parse(text);
  return value as T;
}
