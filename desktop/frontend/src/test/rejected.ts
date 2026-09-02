/**
 * Hold a command that is expected to reject, so a test can start it, drive several more steps,
 * and assert its outcome afterwards. `expect(...).rejects` cannot be deferred like that, and an
 * unhandled rejection between the two points fails the run.
 */
export function rejected(operation: Promise<unknown>): Promise<Error> {
  return operation.then(
    () => {
      throw new Error("operation unexpectedly resolved");
    },
    (error: unknown) => (error instanceof Error ? error : new Error(String(error))),
  );
}
