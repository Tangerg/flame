import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { Activity, Suspense } from "react";
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
  targetKey = "original",
}: {
  acceptsImages?: boolean;
  deliver: (files: File[], intake: ComposerImageIntake) => void;
  targetKey?: string;
}) {
  const { ref, dragging } = useComposerFileDrop(acceptsImages, deliver);
  return (
    <div key={targetKey} ref={ref} data-testid="receiver" data-dragging={dragging ? "" : undefined}>
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

it.each(["Activity", "Suspense"])(
  "clears a retired cue after %s reconnects without acquiring the old drag",
  async (lifetime) => {
    const deliver = vi.fn();
    const pending = new Promise<never>(() => {});
    function Pending(): never {
      throw pending;
    }
    const scene = (visible: boolean) => {
      const receiver = <Receiver deliver={deliver} />;
      return lifetime === "Activity" ? (
        <Activity mode={visible ? "visible" : "hidden"}>{receiver}</Activity>
      ) : (
        <Suspense fallback={<p>Waiting for composer</p>}>
          {!visible && <Pending />}
          {receiver}
        </Suspense>
      );
    };
    const view = client.render(scene(true));
    const original = enter();
    await waitFor(() => expect(original.receiver.hasAttribute("data-dragging")).toBe(true));
    view.rerender(scene(false));
    view.rerender(scene(true));
    expect(screen.getByTestId("receiver")).toBe(original.receiver);
    expect(original.receiver.hasAttribute("data-dragging")).toBe(false);
    fireEvent.dragOver(original.receiver, { dataTransfer: original.transfer });
    fireEvent.drop(original.receiver, { dataTransfer: original.transfer });
    expect(deliver).not.toHaveBeenCalled();

    const next = enter();
    await waitFor(() => expect(next.receiver.hasAttribute("data-dragging")).toBe(true));
    fireEvent.drop(next.receiver, { dataTransfer: next.transfer });
    expect(deliver).toHaveBeenCalledExactlyOnceWith([image], expect.any(Object));
  },
);

it("does not let a successor receiver acquire a drag started by a retired receiver", () => {
  const deliver = vi.fn();
  const view = client.render(<Receiver key="original" deliver={deliver} />);
  const original = enter();
  act(() => client.composer.store.getState().loadSession("successor-draft"));
  view.rerender(<Receiver key="successor" deliver={deliver} />);
  const successor = screen.getByTestId("receiver");
  fireEvent.dragOver(successor, { dataTransfer: original.transfer });
  fireEvent.drop(successor, { dataTransfer: original.transfer });
  expect(deliver).not.toHaveBeenCalled();
  const next = enter();
  fireEvent.drop(next.receiver, { dataTransfer: next.transfer });
  expect(deliver).toHaveBeenCalledExactlyOnceWith([image], expect.any(Object));
});

it("moves the receiver to a rebound surface without granting it the retired surface's drag", async () => {
  const deliver = vi.fn();
  const view = client.render(<Receiver deliver={deliver} />);
  const original = enter();
  await waitFor(() => expect(original.receiver.hasAttribute("data-dragging")).toBe(true));

  view.rerender(<Receiver deliver={deliver} targetKey="replacement" />);
  const replacement = screen.getByTestId("receiver");
  expect(replacement).not.toBe(original.receiver);
  expect(original.receiver.isConnected).toBe(false);
  expect(replacement.hasAttribute("data-dragging")).toBe(false);
  fireEvent.dragOver(replacement, { dataTransfer: original.transfer });
  fireEvent.drop(replacement, { dataTransfer: original.transfer });
  expect(deliver).not.toHaveBeenCalled();

  const next = enter();
  await waitFor(() => expect(next.receiver.hasAttribute("data-dragging")).toBe(true));
  fireEvent.drop(next.receiver, { dataTransfer: next.transfer });
  expect(deliver).toHaveBeenCalledExactlyOnceWith([image], expect.any(Object));
});

it("retires a rebound target even while its original element remains connected", () => {
  const deliver = vi.fn();
  function RetainedSurfaces({ replacement }: { replacement: boolean }) {
    const { ref } = useComposerFileDrop(true, deliver);
    return (
      <>
        <div ref={replacement ? null : ref} data-testid="original-surface">
          Original
        </div>
        <div ref={replacement ? ref : null} data-testid="replacement-surface">
          Replacement
        </div>
      </>
    );
  }
  const view = client.render(<RetainedSurfaces replacement={false} />);
  const original = screen.getByTestId("original-surface");
  const replacement = screen.getByTestId("replacement-surface");
  const retired = dragTransfer([image]);
  fireEvent.dragEnter(original, { dataTransfer: retired, relatedTarget: null });
  fireEvent.dragOver(original, { dataTransfer: retired });

  view.rerender(<RetainedSurfaces replacement />);
  expect(original.isConnected).toBe(true);
  expect(screen.getByTestId("replacement-surface")).toBe(replacement);
  fireEvent.dragOver(original, { dataTransfer: retired });
  fireEvent.drop(original, { dataTransfer: retired });
  expect(deliver).not.toHaveBeenCalled();

  const fresh = dragTransfer([image]);
  fireEvent.dragEnter(replacement, { dataTransfer: fresh, relatedTarget: null });
  fireEvent.dragOver(replacement, { dataTransfer: fresh });
  fireEvent.drop(replacement, { dataTransfer: fresh });
  expect(deliver).toHaveBeenCalledExactlyOnceWith([image], expect.any(Object));

  const returning = dragTransfer([image]);
  fireEvent.dragEnter(replacement, { dataTransfer: returning, relatedTarget: null });
  fireEvent.dragOver(replacement, { dataTransfer: returning });
  view.rerender(<RetainedSurfaces replacement={false} />);
  expect(replacement.isConnected).toBe(true);
  fireEvent.dragOver(original, { dataTransfer: returning });
  fireEvent.drop(original, { dataTransfer: returning });
  expect(deliver).toHaveBeenCalledTimes(1);

  const afterReturn = dragTransfer([image]);
  fireEvent.dragEnter(original, { dataTransfer: afterReturn, relatedTarget: null });
  fireEvent.dragOver(original, { dataTransfer: afterReturn });
  fireEvent.drop(original, { dataTransfer: afterReturn });
  expect(deliver).toHaveBeenCalledTimes(2);
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
