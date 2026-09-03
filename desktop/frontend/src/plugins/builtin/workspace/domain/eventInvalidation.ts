export type WorkspaceInvalidationTarget =
  | "all"
  | "agentMemory"
  | "agentSessionProjection"
  | "approvalMode"
  | "approvalRules"
  | "agentDocs"
  | "diff"
  | "fileHead"
  | "fileList"
  | "fileRead"
  | "filesChanged"
  | "grep"
  | "hooks"
  | "knowledge"
  | "models"
  | "mcpServers"
  | "mcpTools"
  | "pendingWork"
  | "providers"
  | "recipes"
  | "schedules"
  | "sessionUsage"
  | "sessions"
  | "usageSummary"
  | "utilityRole"
  | "embeddingRole"
  | "skills"
  | "managedSkills"
  | "skillProposals";

// Spelled here rather than imported from the wire so this layer stays protocol-free. The
// assignment at the subscribe adapter is then the DRIFT GATE: a signal the runtime adds
// surfaces as a type error at the boundary instead of reaching a default branch.
type WorkspaceEventType =
  | "files.changed"
  | "skills.changed"
  | "mcp.changed"
  | "schedules.changed"
  | "sessions.changed"
  | "runs.changed"
  | "plan.changed"
  | "goals.changed"
  | "interrupts.changed"
  | "knowledge.changed"
  | "hooks.changed"
  | "models.changed"
  | "approvals.changed"
  | "agentMemory.changed"
  | "resync";

type WorkspaceTopic = Exclude<WorkspaceEventType, "resync">;

export interface WorkspaceEventLike {
  type: WorkspaceEventType;
  sequence: number;
  sessionIds?: string[];
  topics?: WorkspaceTopic[];
}

// A signal carries no values, so this mapping is the whole of it — nothing to merge, and
// nothing stale that the next read would not fix.
//
// The switch is EXHAUSTIVE by construction: a topic with no read is a signal this client
// asked for and then dropped, which is indistinguishable from a bug.
export function workspaceInvalidations(ev: WorkspaceEventLike): WorkspaceInvalidationTarget[] {
  switch (ev.type) {
    case "files.changed":
      // Keeping only the VCS projections fresh leaves the open file, the lazy tree,
      // completion, search and file-backed catalogs stale for their whole cache life.
      return [
        "filesChanged",
        "diff",
        "fileList",
        "fileRead",
        "fileHead",
        "grep",
        "recipes",
        "hooks",
        "knowledge",
        "agentDocs",
        "skills",
      ];
    case "skills.changed":
      return ["skills", "managedSkills", "skillProposals"];
    case "mcp.changed":
      return ["mcpServers", "mcpTools"];
    case "schedules.changed":
      return ["schedules"];
    case "sessions.changed":
      return ["sessions"];
    case "runs.changed":
      // Both usage reads are projections of the same durable Run rows, so refreshing only
      // the active Session's chip leaves a mounted cross-session pane stale. Session list
      // and pending work have their OWN signals; invalidating them here duplicates every
      // lifecycle read and races two refetches of one resource.
      return ["sessionUsage", "usageSummary", "agentSessionProjection"];
    case "interrupts.changed":
      return ["agentSessionProjection", "pendingWork"];
    case "goals.changed":
      // Goal is companion material of the mounted Session snapshot: reading it
      // independently splits Plan/HITL/Run/Tool from the autonomous move.
      return ["agentSessionProjection"];
    case "plan.changed":
      return ["agentSessionProjection"];
    case "knowledge.changed":
      return ["knowledge"];
    case "hooks.changed":
      return ["hooks"];
    case "models.changed":
      // Role and provider reads must converge from the SAME committed configuration.
      return ["providers", "models", "utilityRole", "embeddingRole"];
    case "approvals.changed":
      return ["approvalMode", "approvalRules"];
    case "agentMemory.changed":
      return ["agentMemory"];
    case "resync": {
      // `resync` already names every topic folded while this subscriber's queue was full.
      // Widening it to every query creates false dependencies between unrelated read
      // models and can turn one watched Git change into a refetch loop.
      if (!ev.topics?.length) return ["all"];
      const targets = new Set<WorkspaceInvalidationTarget>();
      for (const topic of ev.topics) {
        for (const target of workspaceInvalidations({ type: topic, sequence: ev.sequence })) {
          targets.add(target);
        }
      }
      return [...targets];
    }
    default: {
      const unhandled: never = ev.type;
      return unhandled;
    }
  }
}
