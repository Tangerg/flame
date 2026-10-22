import { type MutationPromise } from "@flame/runtime-client";
import type { ScrollPosition } from "@flame/component-sdk";
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
export type CatalogReturn = { opener: HTMLElement; scroll: ScrollPosition[] };
export type Command = {
  promise: MutationPromise<void>;
  label: string;
  uncertain: boolean;
  error?: string;
};
export type Review = {
  proposal: SkillProposal;
  decision: "approve" | "reject";
  opener: HTMLElement;
};
