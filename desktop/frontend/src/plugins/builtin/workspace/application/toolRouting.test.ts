import { beforeEach, describe, expect, it } from "vitest";
import type { ToolCall } from "@/plugins/sdk/types/agentSessionView";
import {
  useContextDockStore,
  WorkspaceFileFocus,
} from "@/plugins/builtin/workspace/adapters/contextDockStore";
import { navigator } from "@/lib/navigation";
import { hasWorkspaceViewForTool, openWorkspaceViewForTool } from "./toolRouting";

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
  beforeEach(() => {
    navigator().go({ view: null, dock: null });
    useContextDockStore.setState({
      dockViewIds: [],
      lastViewId: null,
      fileFocus: WorkspaceFileFocus.empty(),
    });
  });

  it("reports whether a tool has a workspace view", () => {
    expect(hasWorkspaceViewForTool(toolCall({ id: "t1", name: "shell" }))).toBe(false);
    expect(
      hasWorkspaceViewForTool(
        toolCall({ id: "t2", name: "read", fn: "src/app.ts", fnKind: "path" }),
      ),
    ).toBe(true);
    expect(hasWorkspaceViewForTool(toolCall({ id: "t3", name: "grep" }))).toBe(false);
  });

  it("keeps command tools in the conversation", () => {
    openWorkspaceViewForTool(toolCall({ id: "t1", name: "shell", fn: "ls -la" }));
    expect(navigator().get().dock).toBeNull();
    expect(navigator().get().view).toBeNull();
  });

  it("opens a fileEdit tool as the diff split and focuses its file", () => {
    openWorkspaceViewForTool(
      toolCall({ id: "t2", name: "apply_patch", fn: "src/app.ts", fnKind: "path" }),
    );
    expect(navigator().get().dock).toBe("diff");
    expect(navigator().get().view).toBeNull();
    expect(useContextDockStore.getState().fileFocus).toMatchObject({
      path: "src/app.ts",
      revision: 1n,
    });
  });

  it("does not feed a multi-file patch label to the diff's active-file focus", () => {
    useContextDockStore.getState().focusFile("src/old.ts");
    openWorkspaceViewForTool(toolCall({ id: "t3", name: "apply_patch", fn: "apply_patch" }));
    expect(navigator().get().dock).toBe("diff");
    expect(useContextDockStore.getState().fileFocus).toMatchObject({ path: "", revision: 2n });
  });

  it("promotes no view for inline-only categories", () => {
    openWorkspaceViewForTool(toolCall({ id: "t4", name: "grep", fn: "foo" }));
    expect(navigator().get().dock).toBeNull();
    expect(navigator().get().view).toBeNull();
  });

  it("opens a read tool at its file without showing a diff", () => {
    openWorkspaceViewForTool(
      toolCall({ id: "t5", name: "read", fn: "src/app.ts", fnKind: "path" }),
    );
    expect(navigator().get().dock).toBe("file");
    expect(useContextDockStore.getState().fileViewer).toEqual({ path: "src/app.ts", line: 0 });
  });

  it("offers no file destination without an authoritative path", () => {
    const read = toolCall({ id: "t6", name: "read", fn: "Read file" });
    expect(hasWorkspaceViewForTool(read)).toBe(false);
    openWorkspaceViewForTool(read);
    expect(navigator().get().dock).toBeNull();
  });
});
