import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { useRef } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { dragTransfer } from "@/test/dragTransfer";
import { ContentVisibility } from "@flame/ui";
import { useComposerFileDrop } from "./ComposerFileDrop";
import { type ComposerImageIntake } from "@/features/composer/application/state";
import { RuntimeConnectionGeneration } from "@/features/connection/public/services";
import { useComposer } from "@/features/composer/public/composer";

let client: TestClient;
beforeEach(() => {
  client = createTestClient();
});
afterEach(async () => {
  fireEvent.dragEnd(window, { dataTransfer: dragTransfer() });
  cleanup();
  await client.dispose();
});

function Receiver({
  acceptsImages = true,
  deliver,
}: {
  acceptsImages?: boolean;
  deliver: (files: File[], intake: ComposerImageIntake) => void;
}) {
  const target = useRef<HTMLDivElement>(null);
  const dragging = useComposerFileDrop(target, acceptsImages, deliver);
  return (
    <div ref={target} data-testid="receiver" data-dragging={dragging ? "" : undefined}>
      Files
    </div>
  );
}

const image = new File(["pixels"], "photo.png", { type: "image/png" });
const text = new File(["text"], "notes.txt", { type: "text/plain" });
function enter(transfer = dragTransfer([image])) {
  const receiver = screen.getByTestId("receiver");
  fireEvent.dragEnter(receiver, { dataTransfer: transfer, relatedTarget: null });
  fireEvent.dragOver(receiver, { dataTransfer: transfer });
  return { receiver, transfer };
}

it("shows a generic file cue and delivers only supported image files once through its target", async () => {
  const deliver = vi.fn();
  client.render(<Receiver deliver={deliver} />);
  const { receiver, transfer } = enter(dragTransfer([text, image]));
  await waitFor(() => expect(receiver.hasAttribute("data-dragging")).toBe(true));
  fireEvent.drop(receiver, { dataTransfer: transfer });
  expect(deliver).toHaveBeenCalledExactlyOnceWith(
    [image],
    expect.objectContaining({ current: expect.any(Function), addFiles: expect.any(Function) }),
  );
  expect(receiver.hasAttribute("data-dragging")).toBe(false);
});

it("observes non-image files without claiming that they were attached", async () => {
  const deliver = vi.fn();
  client.render(<Receiver deliver={deliver} />);
  const { receiver, transfer } = enter(dragTransfer([text]));
  await waitFor(() => expect(receiver.hasAttribute("data-dragging")).toBe(true));
  fireEvent.drop(receiver, { dataTransfer: transfer });
  expect(deliver).not.toHaveBeenCalled();
});

it("never delivers a file dropped outside the accepting surface", () => {
  const deliver = vi.fn();
  client.render(
    <>
      <Receiver deliver={deliver} />
      <div data-testid="other">Another target</div>
    </>,
  );
  const { transfer } = enter();
  fireEvent.dragOver(screen.getByTestId("other"), { dataTransfer: transfer });
  fireEvent.drop(screen.getByTestId("other"), { dataTransfer: transfer });
  expect(deliver).not.toHaveBeenCalled();
});

it.each(["Session", "Runtime generation", "endpoint", "capability", "hidden", "unmount"])(
  "retires the old receiver after %s changes and refuses the old drag",
  (reason) => {
    const deliver = vi.fn();
    const scene = (visible = true, acceptsImages = true) => (
      <ContentVisibility visible={visible}>
        <Receiver deliver={deliver} acceptsImages={acceptsImages} />
      </ContentVisibility>
    );
    const view = client.render(scene());
    const { receiver, transfer } = enter();
    if (reason === "Session") act(() => client.composer.store.getState().loadSession("next-draft"));
    else if (reason === "Runtime generation")
      act(() =>
        client.projection.store.setState({
          connectionGeneration: RuntimeConnectionGeneration.forProcess("replacement-runtime"),
        }),
      );
    else if (reason === "endpoint")
      act(() => client.runtime.endpoint.applyRuntimeEndpoint("https://replacement.runtime.test"));
    else if (reason === "capability") view.rerender(scene(true, false));
    else if (reason === "hidden") view.rerender(scene(false));
    else view.unmount();
    fireEvent.dragOver(receiver.isConnected ? receiver : window, { dataTransfer: transfer });
    fireEvent.drop(receiver.isConnected ? receiver : window, { dataTransfer: transfer });
    expect(deliver).not.toHaveBeenCalled();
  },
);

it("admits the next external drag after retiring an earlier Session", () => {
  const deliver = vi.fn();
  client.render(<Receiver deliver={deliver} />);
  const old = enter();
  act(() => client.composer.store.getState().loadSession("next-draft"));
  fireEvent.dragOver(old.receiver, { dataTransfer: old.transfer });
  fireEvent.drop(old.receiver, { dataTransfer: old.transfer });
  const next = enter();
  fireEvent.drop(next.receiver, { dataTransfer: next.transfer });
  expect(deliver).toHaveBeenCalledTimes(1);
});

it("clears cancellation and permits a later drag without a depth counter", async () => {
  const deliver = vi.fn();
  client.render(<Receiver deliver={deliver} />);
  const first = enter();
  await waitFor(() => expect(first.receiver.hasAttribute("data-dragging")).toBe(true));
  fireEvent.dragLeave(first.receiver, { dataTransfer: first.transfer, relatedTarget: null });
  expect(first.receiver.hasAttribute("data-dragging")).toBe(false);
  expect(deliver).not.toHaveBeenCalled();
  const next = enter();
  fireEvent.drop(next.receiver, { dataTransfer: next.transfer });
  expect(deliver).toHaveBeenCalledTimes(1);
});

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
