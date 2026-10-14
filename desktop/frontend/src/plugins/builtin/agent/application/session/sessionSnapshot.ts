import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";
import type { AgentSessionView } from "@/plugins/sdk/types/agentSessionView";
import type { AgentSessionSnapshot } from "../ports/runtimeGateway";
import { reduceDurableItem } from "../fold/reducer";
import { foldRunSnapshot } from "../fold/runSnapshot";
import { foldPendingInterruptSet } from "../fold/pendingInterruptSnapshot";
import { onPlanUpdated } from "../fold/planHandlers";
import { userContentBlocks } from "../fold/projections";

export function projectAgentSessionSnapshot(snapshot: AgentSessionSnapshot): AgentSessionView {
  let view = EMPTY_AGENT_SESSION_VIEW;
  for (const run of snapshot.runs) view = foldRunSnapshot(view, run);
  for (const item of snapshot.items) view = reduceDurableItem(view, item);
  for (const pending of snapshot.pendingInterruptSets) {
    view = foldPendingInterruptSet(view, pending);
  }
  for (const pending of snapshot.pendingSteers ?? []) {
    if (view.messages.some((message) => message.id === pending.userItemId)) continue;
    view = {
      ...view,
      messages: [
        ...view.messages,
        {
          id: pending.userItemId,
          role: "user",
          runId: null,
          blocks: userContentBlocks(
            pending.input.parts.map((part) =>
              part.kind === "text"
                ? { type: "text", text: part.text }
                : { type: "image", mime: part.mime, data: part.data },
            ),
          ),
          steer: { runId: pending.runId, receipt: pending.receipt },
        },
      ],
    };
  }
  if (snapshot.plan) view = onPlanUpdated(view, snapshot.plan);
  return view;
}
