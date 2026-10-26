import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { drainBrowserTasks } from "@/test/browserTasks";
import { SessionRow } from "@/features/sidebar/ui/SessionRow";

function renderRow(title: string) {
  testClient.render(
    <SessionRow
      session={{
        id: "s",
        revision: 1,
        title,
        time: new Date().toISOString(),
        attention: "none",
        favorite: false,
      }}
      active={false}
      onRename={vi.fn()}
      onSelect={vi.fn()}
    />,
  );
}

describe("session row context menu", () => {
  afterEach(async () => {
    cleanup();
    await drainBrowserTasks();
  });

  it("names the menu after the session title", async () => {
    renderRow("Release notes");
    fireEvent.contextMenu(screen.getByText("Release notes"));
    expect(await screen.findByRole("menu", { name: "Release notes" })).toBeTruthy();
  });

  it("names an untitled session's menu after its displayed fallback", async () => {
    renderRow("   ");
    fireEvent.contextMenu(screen.getByText("Untitled session"));
    expect(await screen.findByRole("menu", { name: "Untitled session" })).toBeTruthy();
  });
});
