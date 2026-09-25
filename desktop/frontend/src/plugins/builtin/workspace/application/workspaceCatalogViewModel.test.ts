import { describe, expect, it } from "vitest";
import { workspaceSkillsViewModel } from "./workspaceCatalogViewModel";

describe("workspace catalog view models", () => {
  it("gates skills rows when the runtime capability is off", () => {
    expect(
      workspaceSkillsViewModel(
        [{ name: "review", description: "Review code", scope: "project" as const }],
        false,
      ),
    ).toMatchObject({
      rows: [],
      enabled: false,
      isEmpty: true,
    });
  });

  it("projects skills into stable rows", () => {
    expect(
      workspaceSkillsViewModel(
        [{ name: "review", description: "Review code", scope: "project" as const }],
        true,
      ).rows,
    ).toEqual([{ id: "review", name: "review", description: "Review code", scope: "project" }]);
  });
});
