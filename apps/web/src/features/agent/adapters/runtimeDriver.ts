import { AgentRunStartOptions } from "@/features/agent/public/runOptions";
import {
  type ContentBlock,
  type InterruptResponse,
  type ItemId,
  type RunId,
  type SegmentId,
  type RunOpeningResult,
} from "@flame/runtime-client";
export interface RuntimeAgentDriver {
  start: (
    input: ContentBlock[],
    options: AgentRunStartOptions,
    signal?: AbortSignal,
    onMutation?: (idempotencyKey: string) => void,
  ) => Promise<RunOpeningResult<{ runId: RunId; segmentId: SegmentId; userItemId: ItemId }>>;
  resume: (
    runId: RunId,
    options: {
      responses: InterruptResponse[];
      input?: ContentBlock[];
    },
    signal?: AbortSignal,
  ) => Promise<RunOpeningResult<{ runId: RunId; segmentId: SegmentId; userItemId?: ItemId }>>;
}

export interface AgentSourceSpec {
  id: string;
  label: string;
  priority?: number;
  factory: () => RuntimeAgentDriver;
}
