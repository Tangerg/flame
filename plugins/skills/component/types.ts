import { type MutationPromise } from "@flame/runtime-client";
import {
  type ManagedSkill,
  type Page,
  type Skill,
  type SkillDetail,
  type SkillDiscovery,
  type SkillProposal,
} from "@flame/runtime-client/wire";
export type SkillSnapshot =
  | { type: "available"; data: SkillDiscovery }
  | { type: "library"; data: Page<ManagedSkill> }
  | { type: "review"; data: Page<SkillProposal> };
export type SkillRecord = Skill | ManagedSkill | SkillProposal;
export type Target = SkillSnapshot["type"];
export type Read = {
  loading: boolean;
  accepted?: SkillSnapshot;
  error?: string;
};
export type OpenedDocument = {
  name: string;
  loading: boolean;
  accepted?: SkillDetail;
  error?: string;
};
export type CatalogReturn = { opener: HTMLElement; scrollTop: number };
export type CommandIntent = {
  action: "approve" | "reject" | "archive" | "restore";
  name: string;
};
export type Command = {
  promise: MutationPromise<void>;
  intent: CommandIntent;
};
// One command at a time. An uncertain settlement keeps its original command
// until a check through the same idempotency key settles it.
export type CommandState =
  | { phase: "submitting"; command: Command }
  | { phase: "uncertain"; command: Command; error: string };
export type Notice =
  | { kind: "completed"; intent: CommandIntent }
  | { kind: "revisionConflict"; intent: CommandIntent }
  | { kind: "failure"; intent: CommandIntent; detail: string };
export type Review = {
  proposal: SkillProposal;
  decision: "approve" | "reject";
  opener: HTMLElement;
};
