import type {
  ManagedSkill,
  Page,
  SkillDetail,
  SkillDiscovery,
  SkillProposal,
} from "@flame/runtime-contract/wire";
import { VISUAL_SESSION_ID } from "./agentSessionSnapshots";

export const VISUAL_SKILL_DOCUMENTS: SkillDetail[] = [
  {
    name: "risk-first-review",
    description: "Review the riskiest change first.",
    scope: "user",
    path: "/Users/visual/.flame/skills/risk-first-review/SKILL.md",
    revision: "1".repeat(64),
    instructions: "Start from the riskiest hunk.",
  },
  {
    name: "runtime-boundaries",
    description: "Keep durable decisions with their Runtime owner.",
    scope: "project",
    path: "/Users/visual/scope/.flame/skills/runtime-boundaries/SKILL.md",
    revision: "2".repeat(64),
    instructions: "Trace each fact to its semantic owner before editing.",
  },
];
export const VISUAL_SKILL_DISCOVERY: SkillDiscovery = {
  skills: VISUAL_SKILL_DOCUMENTS.map(({ name, description, scope }) => ({
    name,
    description,
    scope,
  })),
  diagnostics: [],
};
export const VISUAL_SKILL_LIBRARY: Page<ManagedSkill> = {
  data: [
    {
      name: "risk-first-review",
      description: "Review the riskiest change first.",
      lifecycle: "active",
    },
    { name: "old-format-notes", description: "Retained personal notes.", lifecycle: "archived" },
  ],
};
export const VISUAL_SKILL_PROPOSALS: Page<SkillProposal> = {
  data: [
    {
      name: "risk-first-review",
      description: "Proposed review instructions.",
      scope: "project",
      origin: "requested",
      revises: true,
      revision: "3".repeat(64),
      sourceSession: VISUAL_SESSION_ID,
      instructions: "Start from the riskiest hunk.",
    },
    {
      name: "release-notes",
      description: "Summarize verified product behavior.",
      scope: "project",
      origin: "mined",
      revises: false,
      revision: "4".repeat(64),
      sourceSession: VISUAL_SESSION_ID,
      instructions: "Describe the final behavior and its verification.",
    },
  ],
};
