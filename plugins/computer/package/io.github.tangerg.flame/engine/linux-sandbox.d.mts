export class SandboxRefusal extends Error {}

export function decide(
  args: readonly string[],
  environment: Readonly<Record<string, string | undefined>>,
  parentExecutable: string,
): { command: string; args: string[] };
