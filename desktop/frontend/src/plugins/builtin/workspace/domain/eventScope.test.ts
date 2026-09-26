import { describe, expect, it } from "vitest";
import { workspaceQueryAffected, type WorkspaceEventLike } from "./eventInvalidation";

const scopes = [
  { watchId: "first", workspace: { path: "/first" }, cwd: "/linked/first" },
  { watchId: "second", workspace: { path: "/second" }, cwd: "/second" },
];

describe("workspace query invalidation scope", () => {
  const event: WorkspaceEventLike = {
    type: "files.changed",
    sequence: 1,
    workspace: { path: "/first" },
    paths: ["src/same.ts"],
    watchScopes: scopes,
  };

  it("refreshes the changed file and ancestor lists, with path-component boundaries", () => {
    expect(workspaceQueryAffected("fileRead", { cwd: "/first", path: "src/same.ts" }, event)).toBe(
      true,
    );
    expect(
      workspaceQueryAffected("fileRead", { cwd: "/linked/first", path: "src/same.ts" }, event),
    ).toBe(true);
    expect(workspaceQueryAffected("fileRead", { cwd: "/second", path: "src/same.ts" }, event)).toBe(
      false,
    );
    expect(workspaceQueryAffected("fileRead", { cwd: "/first", path: "src/same.tsx" }, event)).toBe(
      false,
    );
    expect(workspaceQueryAffected("fileList", { cwd: "/first", path: "." }, event)).toBe(true);
    expect(workspaceQueryAffected("fileList", { cwd: "/first", path: "src" }, event)).toBe(true);
    expect(workspaceQueryAffected("fileList", { cwd: "/first", path: "other" }, event)).toBe(false);
    expect(workspaceQueryAffected("diff", { cwd: "/first" }, event)).toBe(true);
    expect(workspaceQueryAffected("diff", { cwd: "/first", path: "other.ts" }, event)).toBe(false);
  });

  it("treats a directory notice as a subtree invalidation for open files", () => {
    expect(
      workspaceQueryAffected(
        "fileRead",
        { cwd: "/first", path: "src/same.ts" },
        { ...event, paths: ["src"] },
      ),
    ).toBe(true);
  });

  it("widens missing or untrustworthy paths only within the known workspace", () => {
    for (const paths of [undefined, [], ["../unknown"]]) {
      expect(
        workspaceQueryAffected(
          "fileRead",
          { cwd: "/first", path: "other.ts" },
          { ...event, paths },
        ),
      ).toBe(true);
      expect(
        workspaceQueryAffected(
          "fileRead",
          { cwd: "/second", path: "other.ts" },
          { ...event, paths },
        ),
      ).toBe(false);
    }
  });

  it("uses Git watch IDs and widens unknown/overflow identities safely", () => {
    const resync: WorkspaceEventLike = {
      type: "resync",
      sequence: 2,
      topics: ["files.changed"],
      watchIds: ["second"],
      watchScopes: scopes,
    };
    expect(workspaceQueryAffected("fileRead", { cwd: "/first" }, resync)).toBe(false);
    expect(workspaceQueryAffected("fileRead", { cwd: "/second" }, resync)).toBe(true);
    expect(
      workspaceQueryAffected("fileRead", { cwd: "/first" }, { ...resync, watchIds: ["unknown"] }),
    ).toBe(true);
    expect(
      workspaceQueryAffected("fileRead", { cwd: "/first" }, { ...resync, watchIds: undefined }),
    ).toBe(true);
  });

  it("keeps user Skill invalidations global in direct and coalesced events", () => {
    expect(
      workspaceQueryAffected("skills", { cwd: "/second" }, { type: "skills.changed", sequence: 1 }),
    ).toBe(true);
    expect(
      workspaceQueryAffected(
        "skills",
        { cwd: "/second" },
        {
          type: "resync",
          sequence: 1,
          topics: ["files.changed", "skills.changed"],
          watchIds: ["first"],
          watchScopes: scopes,
        },
      ),
    ).toBe(true);
    expect(workspaceQueryAffected("managedSkills", undefined, event)).toBe(true);
  });
});
