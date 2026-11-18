import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { describe, expect, it } from "vitest";
import { DEFAULT_ACCENT } from "@/features/theme/kit/appearance";
import { DEFAULT_ACCENTS } from "@/features/theme/defaultAccents";
import { defaultCommands } from "@/features/command/defaultCommands";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { workbenchChat } from "@/features/shell/workbench/index";
import { loadPluginsForTest } from "@/test/kernel";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});

describe("DEFAULT_ACCENTS", () => {
  it("leads with the default accent so choosing it restores the default", () => {
    expect(DEFAULT_ACCENTS[0]!.color).toBe(DEFAULT_ACCENT);
  });
});

describe("defaultStaticCommands", () => {
  it("leaves the global Composer focus command to the installed workbench conversation", async () => {
    await loadPluginsForTest(
      defaultCommands(
        testClient.agent.access,
        testClient.composer.access,
        testClient.workspace.access,
        testClient.runtime,
        testClient.directory.access,
        testClient.navigation,
        testClient.theme.access,
      ),
      workbenchChat,
    );

    expect(
      lookupExtensionPoint(COMMAND)
        .filter((command) => command.id === "composer.focus")
        .map(({ id, label, combo }) => ({ id, label, combo })),
    ).toEqual([{ id: "composer.focus", label: "command.focusComposer", combo: "Mod+L" }]);
  });
});
