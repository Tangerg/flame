import type { WorkspaceSkill } from "./workspaceQueries";

export interface WorkspaceCatalogViewModel<Row> {
  rows: Row[];
  count: number;
  enabled: boolean;
  isEmpty: boolean;
}

export interface WorkspaceSkillRowViewModel {
  id: string;
  name: string;
  description: string;
  scope: "project" | "user";
}

function catalog<Row>(rows: Row[], enabled = true): WorkspaceCatalogViewModel<Row> {
  return {
    rows,
    count: rows.length,
    enabled,
    isEmpty: rows.length === 0,
  };
}

export function workspaceSkillsViewModel(
  skills: readonly WorkspaceSkill[],
  enabled: boolean,
): WorkspaceCatalogViewModel<WorkspaceSkillRowViewModel> {
  if (!enabled) {
    return catalog([], false);
  }

  return catalog(
    skills.map((skill) => ({
      id: skill.name,
      name: skill.name,
      description: skill.description,
      scope: skill.scope,
    })),
  );
}
