export type WorkspaceInvalidationTarget =
  | "all"
  | "agentMemory"
  | "agentSessionProjection"
  | "approvalMode"
  | "approvalRules"
  | "diff"
  | "fileList"
  | "fileRead"
  | "filesChanged"
  | "hooks"
  | "models"
  | "mcpServers"
  | "mcpTools"
  | "providers"
  | "schedules"
  | "sessionUsage"
  | "sessions"
  | "usageSummary"
  | "utilityRole"
  | "embeddingRole"
  | "skills"
  | "managedSkills"
  | "skillProposals";

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

export function workspaceInvalidations(ev: WorkspaceEventLike): WorkspaceInvalidationTarget[] {
  switch (ev.type) {
    case "files.changed":
      return ["filesChanged", "diff", "fileList", "fileRead", "hooks", "skills"];
    case "skills.changed":
      return ["skills", "managedSkills", "skillProposals"];
    case "mcp.changed":
      return ["mcpServers", "mcpTools"];
    case "schedules.changed":
      return ["schedules"];
    case "sessions.changed":
      return ["sessions"];
    case "runs.changed":
      return ["sessionUsage", "usageSummary", "agentSessionProjection"];
    case "interrupts.changed":
      return ["agentSessionProjection"];
    case "goals.changed":
      return ["agentSessionProjection"];
    case "plan.changed":
      return ["agentSessionProjection"];
    case "hooks.changed":
      return ["hooks"];
    case "models.changed":
      return ["providers", "models", "utilityRole", "embeddingRole"];
    case "approvals.changed":
      return ["approvalMode", "approvalRules"];
    case "agentMemory.changed":
      return ["agentMemory"];
    case "resync": {
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
