import { beforeEach, expect, it, onTestFinished, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/react";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { ComposerAttachments } from "./ComposerAttachments";
let client: TestClient;
beforeEach(() => {
  client = createTestClient();
  onTestFinished(() => client.dispose());
});
it("keeps attachment identity and removal independent from editor text", () => {
  const remove = vi.fn();
  client.render(
    <ComposerAttachments
      images={[{ id: "pending", name: "capture.png", status: "pending" }]}
      pastes={[]}
      onRemoveImage={remove}
      onRemovePaste={vi.fn()}
      onEditPaste={() => true}
      onRestorePaste={vi.fn()}
      composerInput={{ current: null }}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "Remove image" }));
  expect(remove).toHaveBeenCalledExactlyOnceWith("pending");
});
