export interface AgentOpeningAcceptance {
  readonly status: "accepted" | "retired";
  readonly sessionId: string;
  readonly runId: string;
  readonly userItemId?: string;
}

export type AgentInputSettlement =
  | {
      readonly status: "accepted";
      readonly sessionId: string;
      readonly runId: string;
      readonly userItemId: string;
    }
  | {
      readonly status: "retired";
      readonly sessionId: string;
      readonly runId: string;
      readonly userItemId?: string;
    }
  | {
      readonly status: "unknown";
      readonly idempotencyKey: string;
      recover(): Promise<AgentInputSettlement>;
    };

export function acceptedAgentInput(result: AgentOpeningAcceptance): AgentInputSettlement {
  if (result.status === "retired") return { ...result, status: "retired" };
  if (!result.userItemId) throw new Error("accepted input omitted its user item identity");
  return { ...result, status: "accepted", userItemId: result.userItemId };
}

export function settleAgentInput(
  operation: Promise<AgentOpeningAcceptance>,
  original: {
    key(): string | undefined;
    isUnknown(error: unknown): boolean;
    recover(): Promise<AgentInputSettlement>;
  },
): Promise<AgentInputSettlement> {
  const settlement = operation.then(acceptedAgentInput, (error: unknown): AgentInputSettlement => {
    const idempotencyKey = original.key();
    if (!idempotencyKey || !original.isUnknown(error)) throw error;
    return { status: "unknown", idempotencyKey, recover: original.recover };
  });
  // Composer admission deliberately does not wait for the receipt. Its existing
  // command owner reports failures while other consumers can await this handle.
  void settlement.catch(() => {});
  return settlement;
}
