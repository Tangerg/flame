import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { openWorkspaceViewAtPath } from "@/features/workspace/public/navigation";
import { beforeEach, describe, expect, it } from "vitest";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { WorkspaceFileFocus } from "@/features/workspace/adapters/contextDockStore";

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
    testClient.navigation.go({ view: null, dock: null });
    testClient.workspace.dock.store.setState({
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
      testClient.workspace.access,
      toolCall({ id: "t1", name: "shell", category: "shell", fn: "ls -la" }),
    );
    expect(testClient.navigation.get().dock).toBeNull();
    expect(testClient.navigation.get().view).toBeNull();
  });

  it("opens the captured contributed view at the recorded path", () => {
    openWorkspaceViewAtPath(testClient.workspace.access, "package:review:diff", "src/app.ts");
    expect(testClient.navigation.get().dock).toBe("package:review:diff");
    expect(testClient.workspace.dock.store.getState().fileFocus.path).toBe("src/app.ts");
  });

  it("promotes no view for inline-only categories", () => {
    openWorkspaceViewForTool(
      testClient.workspace.access,
      toolCall({ id: "t4", name: "grep", category: "search", fn: "foo" }),
    );
    expect(testClient.navigation.get().dock).toBeNull();
    expect(testClient.navigation.get().view).toBeNull();
  });

  it("opens a read tool at its file without showing a diff", () => {
    openWorkspaceViewForTool(
      testClient.workspace.access,
      toolCall({ id: "t5", name: "read", category: "read", fn: "src/app.ts", fnKind: "path" }),
    );
    expect(testClient.navigation.get().dock).toBe("file");
    expect(testClient.workspace.dock.store.getState().fileViewer).toEqual({
      path: "src/app.ts",
      line: 0,
    });
  });

  it("offers no file destination without an authoritative path", () => {
    const read = toolCall({ id: "t6", name: "read", category: "read", fn: "Read file" });
    expect(hasWorkspaceViewForTool(read)).toBe(false);
    openWorkspaceViewForTool(testClient.workspace.access, read);
    expect(testClient.navigation.get().dock).toBeNull();
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
    openWorkspaceViewForTool(testClient.workspace.access, extensionRead);
    expect(testClient.navigation.get().dock).toBe("file");
    expect(testClient.workspace.dock.store.getState().fileViewer).toEqual({
      path: "src/extension.ts",
      line: 0,
    });
    expect(hasWorkspaceViewForTool(toolCall({ id: "t8", name: "apply_patch" }))).toBe(false);
  });
});
