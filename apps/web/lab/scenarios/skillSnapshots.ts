import {
  type ManagedSkill,
  type Page,
  type SkillDetail,
  type SkillDiscovery,
  type SkillProposal,
} from "@flame/runtime-client/wire";
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
    instructions: [
      "Trace each fact to its semantic owner before editing.",
      "",
      "1. Name the owner first. Session, Run, Goal and Plan belong to Runtime; the CLI and Desktop only project them. 在修改任何状态之前，先说明哪一个模块拥有这个事实，以及其他表示只是投影、缓存还是渲染。",
      "2. Read the declared contract against its implementation. A handled failure whose only report is a tracing span reaches nobody. 如果错误只写进了追踪记录，用户和调用方都看不到它，这本身就是一个缺陷。",
      "3. Keep one transition path. When two representations can both advance the same fact, repair the ownership instead of adding an arbitration rule. 不要用“哪个值优先”的规则掩盖双重所有权。",
      "",
      "Verification:",
      "- Exercise the public binding or protocol, not a private helper.",
      "- Cover restart and recovery when the change touches persisted state. 涉及持久化数据时，同时验证重启与恢复路径。",
      "- Report passed, failed and not-run checks separately; 不要把未运行的检查写成已通过。",
    ].join("\n"),
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
