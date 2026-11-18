import { type MutationPromise, type MutationAttemptOptions } from "@flame/runtime-client";
// One scripted attempt lets UI tests control settlement; transport tests own SDK replay policy.
export function mutationResult<T>(
  execute: (key: string, options: MutationAttemptOptions) => Promise<T>,
  key: string = crypto.randomUUID(),
  options: MutationAttemptOptions = {},
): MutationPromise<T> {
  return Object.assign(
    Promise.resolve().then(() => execute(key, options)),
    {
      idempotencyKey: key,
      retry: (next: MutationAttemptOptions = options) => mutationResult(execute, key, next),
    },
  );
}
