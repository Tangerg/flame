export interface InvokeDiagnosticToolInput {
  name: string;
  arguments: Record<string, unknown>;
  cwd?: string;
}

export interface DiagnosticToolGateway {
  invoke(input: InvokeDiagnosticToolInput): Promise<unknown>;
}
