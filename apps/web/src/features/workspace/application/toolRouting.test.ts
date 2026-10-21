import { contributeForTest } from "@/test/kernel";
import { WORKSPACE_VIEW } from "@/features/workspace/public/views";
import { beforeEach, describe, expect, it } from "vitest";
import { type ToolCall } from "@/features/agent/public/sessionView";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/features/workspace/adapters/contextDockStore";
import { navigator } from "@/lib/navigation";
import {
  hasWorkspaceViewForTool,
  openWorkspaceViewForTool,
} from "@/features/workspace/application/toolRouting";
const toolCall = ({
  runId = "run_1",
  ...over
}: Partial<ToolCall> & Pick<ToolCall, "id" | "name">): ToolCall => ({
  runId,
  fn: "",
  args: "",
  status: "ok",
  ...over,
});

describe("openWorkspaceViewForTool", () => {
  beforeEach(async () => {
    await contributeForTest((ctx) =>
      ctx.contribute(WORKSPACE_VIEW, {
        id: "package:review:diff",
        title: "Diff",
        material: "diff",
        dock: "workspace",
        component: () => null,
      }),
    );
    navigator().go({ view: null, dock: null });
    useContextDockStore.setState({
      dockViewIds: [],
      lastViewId: null,
      fileFocus: WorkspaceFileFocus.empty(),
    });
  });

  it("reports whether a tool has a workspace view", () => {
    expect(hasWorkspaceViewForTool(toolCall({ id: "t1", name: "shell", category: "shell" }))).toBe(
      false,
    );
    expect(
      hasWorkspaceViewForTool(
        toolCall({ id: "t2", name: "read", category: "read", fn: "src/app.ts", fnKind: "path" }),
      ),
    ).toBe(true);
    expect(hasWorkspaceViewForTool(toolCall({ id: "t3", name: "grep", category: "search" }))).toBe(
      false,
    );
  });

  it("keeps command tools in the conversation", () => {
    openWorkspaceViewForTool(
      toolCall({ id: "t1", name: "shell", category: "shell", fn: "ls -la" }),
    );
    expect(navigator().get().dock).toBeNull();
    expect(navigator().get().view).toBeNull();
  });

  it("opens a fileEdit tool as the diff split and focuses its file", () => {
    openWorkspaceViewForTool(
      toolCall({
        id: "t2",
        name: "apply_patch",
        category: "edit",
        fn: "src/app.ts",
        fnKind: "path",
      }),
    );
    expect(navigator().get().dock).toBe("package:review:diff");
    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState().fileFocus).toMatchObject({
      path: "src/app.ts",
      revision: 1n,
    });
  });

  it("does not feed a multi-file patch label to the diff's active-file focus", () => {
    useContextDockStore.getState().focusFile("src/old.ts");
    openWorkspaceViewForTool(
      toolCall({ id: "t3", name: "apply_patch", category: "edit", fn: "apply_patch" }),
    );
    expect(navigator().get().dock).toBe("package:review:diff");
    expect(useContextDockStore.getState().fileFocus).toMatchObject({ path: "", revision: 2n });
  });

  it("promotes no view for inline-only categories", () => {
    openWorkspaceViewForTool(toolCall({ id: "t4", name: "grep", category: "search", fn: "foo" }));
    expect(navigator().get().dock).toBeNull();
    expect(navigator().get().view).toBeNull();
  });

  it("opens a read tool at its file without showing a diff", () => {
    openWorkspaceViewForTool(
      toolCall({ id: "t5", name: "read", category: "read", fn: "src/app.ts", fnKind: "path" }),
    );
    expect(navigator().get().dock).toBe("file");
    expect(useContextDockStore.getState().fileViewer).toEqual({ path: "src/app.ts", line: 0 });
  });

  it("offers no file destination without an authoritative path", () => {
    const read = toolCall({ id: "t6", name: "read", category: "read", fn: "Read file" });
    expect(hasWorkspaceViewForTool(read)).toBe(false);
    openWorkspaceViewForTool(read);
    expect(navigator().get().dock).toBeNull();
  });

  it("routes the projected category independently of the tool name", () => {
    const extensionRead = toolCall({
      id: "t7",
      name: "extension.inspect",
      category: "read",
      fn: "src/extension.ts",
      fnKind: "path",
    });
    expect(hasWorkspaceViewForTool(extensionRead)).toBe(true);
    openWorkspaceViewForTool(extensionRead);
    expect(navigator().get().dock).toBe("file");
    expect(useContextDockStore.getState().fileViewer).toEqual({
      path: "src/extension.ts",
      line: 0,
    });
    expect(hasWorkspaceViewForTool(toolCall({ id: "t8", name: "apply_patch" }))).toBe(false);
  });
});
