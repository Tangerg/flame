import { EMPTY_AGENT_SESSION_VIEW } from "@/features/agent/public/sessionView";
import { type AgentSessionView } from "@/features/agent/public/sessionView";
import { type AgentSessionSnapshot } from "@/features/agent/application/ports/runtimeGateway";
import { reduceDurableItem } from "@/features/agent/application/fold/reducer";
import { foldRunSnapshot } from "@/features/agent/application/fold/runSnapshot";
import { foldPendingInterruptSet } from "@/features/agent/application/fold/pendingInterruptSnapshot";
import { onPlanUpdated } from "@/features/agent/application/fold/planHandlers";
import { userContentBlocks } from "@/features/agent/application/fold/projections";
export function projectAgentSessionSnapshot(snapshot: AgentSessionSnapshot): AgentSessionView {
  let view = EMPTY_AGENT_SESSION_VIEW;
  for (const run of snapshot.runs) view = foldRunSnapshot(view, run);
  for (const item of snapshot.items) view = reduceDurableItem(view, item);
  for (const pending of snapshot.pendingInterruptSets) {
    view = foldPendingInterruptSet(view, pending);
  }
  for (const accepted of snapshot.steerInputs ?? []) {
    const identity =
      accepted.receipt === "retired" ? `retired-steer:${accepted.userItemId}` : accepted.userItemId;
    const present = view.messages.find((message) => message.id === identity);
    if (present) {
      if (present.role !== "user" || present.runId !== accepted.runId)
        throw new Error("agent.snapshot.steerInputOwnerMismatch");
      view = {
        ...view,
        messages: view.messages.map((message) =>
          message.id === identity
            ? { ...message, steer: { runId: accepted.runId, receipt: accepted.receipt } }
            : message,
        ),
      };
      continue;
    }
    view = {
      ...view,
      messages: [
        ...view.messages,
        {
          id: identity,
          role: "user",
          runId: null,
          blocks: userContentBlocks(
            accepted.input.parts.map((part) =>
              part.kind === "text"
                ? { type: "text", text: part.text }
                : { type: "image", mime: part.mime, data: part.data },
            ),
          ),
          steer: {
            runId: accepted.runId,
            receipt: accepted.receipt,
            ...(accepted.receipt === "accepted" &&
            view.runsById[accepted.runId]?.status === "finished"
              ? { application: "notApplied" as const }
              : {}),
          },
        },
      ],
    };
  }
  if (snapshot.plan) view = onPlanUpdated(view, snapshot.plan);
  return view;
}
