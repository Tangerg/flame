export class KernelRefusal extends Error {}

export function decide(
  args: readonly string[],
  environment: Readonly<Record<string, string | undefined>>,
): { command: string; args: string[] };
