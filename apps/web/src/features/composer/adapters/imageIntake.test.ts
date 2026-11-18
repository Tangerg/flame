import { act } from "@testing-library/react";
import { afterEach, beforeEach, expect, it } from "vitest";
import { useComposer } from "@/features/composer/public/composer";
import { createTestClient, type TestClient } from "@/test/renderClient";

let client: TestClient;
beforeEach(() => {
  client = createTestClient();
});
afterEach(() => client.dispose());

const image = new File(["pixels"], "photo.png", { type: "image/png" });

it("keeps a captured image intake retired even when the original Session returns", () => {
  const { result } = client.renderHook(() => useComposer().useImageIntake());
  const original = result.current;
  const session = client.composer.store.getState().composer.activeSessionId;
  act(() => {
    client.composer.store.getState().loadSession("next-draft");
    client.composer.store.getState().loadSession(session);
  });
  expect(original.current()).toBe(false);
  act(() => original.addFiles([image]));
  expect(client.composer.store.getState().composer.draft.imageAttachments).toHaveLength(0);
  expect(result.current.current()).toBe(true);
});
