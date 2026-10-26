import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, fireEvent, screen } from "@testing-library/react";
import { ContentVisibility } from "@flame/ui";
import { afterEach, expect, it } from "vitest";
import chatSearch from "@/features/chat/search/index";
import { openChatSearch } from "@/features/chat/search/application/openChatSearch";
import { MESSAGE_CONTENT_SELECTOR } from "@/features/chat/public/message/rendering";
import { Slot } from "@/extensions/host/Slot";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";

afterEach(() => resetKernelForTest());

it("contributes Find to its visible Conversation lifetime without a global overlay", async () => {
  await loadPluginsForTest(chatSearch);
  function Workspace({ visible = true }: { visible?: boolean }) {
    return (
      <>
        <ContentVisibility visible={visible}>
          <div role="region" aria-label="Conversation" tabIndex={-1}>
            <Slot name="chat.search" />
            <article className={MESSAGE_CONTENT_SELECTOR.slice(1)}>Alpha beta</article>
          </div>
        </ContentVisibility>
        <aside aria-label="Global overlays">
          <Slot name="app.overlay" />
        </aside>
        <button type="button">Material</button>
      </>
    );
  }
  const { rerender } = testClient.render(<Workspace />);
  const region = screen.getByRole("region", { name: "Conversation" });
  expect(region.children).toHaveLength(1);
  act(() => openChatSearch());
  const search = screen.getByRole("search", { name: "Search in chat" });
  expect(region.contains(search)).toBe(true);
  expect(screen.getByLabelText("Global overlays").children).toHaveLength(0);
  const material = screen.getByRole("button", { name: "Material" });
  material.focus();
  rerender(<Workspace visible={false} />);
  expect(screen.queryByRole("search")).toBeNull();
  expect(document.activeElement).toBe(material);
  act(() => openChatSearch());
  expect(screen.queryByRole("search")).toBeNull();
  rerender(<Workspace />);
  expect(region.children).toHaveLength(1);
  act(() => openChatSearch());
  fireEvent.click(screen.getByRole("button", { name: "Close" }));
  expect(region.children).toHaveLength(1);
});
