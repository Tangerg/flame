import { beforeEach, expect, it, onTestFinished, vi } from "vitest";
import { act, fireEvent, screen, waitFor } from "@testing-library/react";
import { parseHotkey } from "@tanstack/hotkeys";
import { ContentVisibility } from "@flame/ui";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { loadPluginsForTest } from "@/test/kernel";
import { definePlugin } from "@/extensions/definePlugin";
import { COMPOSER_KEY_BINDING } from "@/features/composer/public/extensionPoints";
import { Composer } from "./Composer";
let client: TestClient;
beforeEach(() => {
  client = createTestClient();
  onTestFinished(() => client.dispose());
});
const props = {
  onSend: () => true,
  images: [],
  pastes: [],
  onRemoveImage() {},
  onAddImages() {},
  onRemovePaste() {},
  onEditPaste: () => true,
  onRestorePaste() {},
  onAddPaste() {},
  acceptsImages: true,
};
async function mount(text = "captured draft") {
  client.composer.access.replaceDraft({ text });
  const view = client.render(
    <ContentVisibility visible>
      <Composer {...props} />
    </ContentVisibility>,
  );
  const input = await screen.findByRole("textbox", { name: "Message composer" });
  await waitFor(() => expect(input.textContent).toBe(text));
  const writer = client.renderHook(() => client.composer.access.useSetText());
  return { ...view, input, setText: writer.result.current };
}
it("owns its document in Lexical and refuses late consumption of a changed revision", async () => {
  const { input, setText } = await mount();
  expect(input.tagName).toBe("DIV");
  expect(input.getAttribute("contenteditable")).toBe("true");
  const accepted = client.composer.access.captureSubmission();
  act(() => setText("newer editor content"));
  await waitFor(() => expect(input.textContent).toBe("newer editor content"));
  expect(accepted.consume()).toBe(false);
  expect(client.composer.access.captureSubmission().value).toBe("newer editor content");
});
it("clears the exact accepted document and keeps its one-way checkpoint", async () => {
  const { input } = await mount();
  act(() => {
    expect(client.composer.access.captureSubmission().consume()).toBe(true);
  });
  await waitFor(() => expect(input.textContent).toBe(""));
  expect(client.composer.access.initialDocument().text).toBe("");
  expect(client.composer.access.initialDocument().document).toContain('"root"');
});
it("leaves Enter to editing and dispatches the explicit modified binding outside composition", async () => {
  const send = vi.fn(() => true);
  await loadPluginsForTest(
    definePlugin({
      name: "test.composer.keys",
      setup(ctx) {
        ctx.contributions.add(COMPOSER_KEY_BINDING, { key: "Mod+Enter", handler: send });
      },
    }),
  );
  const { input } = await mount();
  input.focus();
  fireEvent.keyDown(input, { key: "Enter" });
  expect(send).not.toHaveBeenCalled();
  const parsed = parseHotkey("Mod+Enter");
  fireEvent.keyDown(input, {
    key: "Enter",
    ctrlKey: parsed.ctrl,
    metaKey: parsed.meta,
    isComposing: true,
  });
  expect(send).not.toHaveBeenCalled();
  fireEvent.keyDown(input, { key: "Enter", ctrlKey: parsed.ctrl, metaKey: parsed.meta });
  expect(send).toHaveBeenCalledOnce();
});
it("makes a retained hidden editor read-only", async () => {
  const { rerender, input } = await mount();
  rerender(
    <ContentVisibility visible={false}>
      <Composer {...props} />
    </ContentVisibility>,
  );
  await waitFor(() => expect(input.getAttribute("contenteditable")).toBe("false"));
});
