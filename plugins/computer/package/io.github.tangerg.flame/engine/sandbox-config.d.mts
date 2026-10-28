export const CONFIGURATION: "FLAME_COMPUTER_SANDBOX";

export interface SandboxConfiguration {
  readonly runtime: string;
  readonly node: string;
  readonly codex: string;
  readonly workdir: string;
  readonly tempDirectory: string;
  readonly guardDirectory: string;
  readonly launcher: string;
  readonly sentinel: string;
  readonly filesystem: Readonly<Record<string, "read" | "write">>;
}

export function configuration(
  environment: Readonly<Record<string, string | undefined>>,
): SandboxConfiguration;

export function trustedServices(
  environment: Readonly<Record<string, string | undefined>>,
  config: SandboxConfiguration,
): { readonly sky: string; readonly files: readonly string[] };
