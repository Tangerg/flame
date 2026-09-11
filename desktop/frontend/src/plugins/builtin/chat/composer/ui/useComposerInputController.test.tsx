import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Everything the controller reaches for that is not its own decision. The classifier it calls —
// `composerCompositionKeyIntent` — is deliberately REAL and has its own tests; what is under
// test here is the lifecycle around it, which is the half that decides what the classifier is
// ever told.
const submitted = vi.fn(() => true);

vi.mock("@/lib/i18n", () => ({ useT: () => (key: string) => key }));
vi.mock("@/plugins/builtin/agent/public/session", () => ({
  useActiveSessionWorkspace: () => ({ status: "ready", cwd: "/w" }),
}));
vi.mock("@/plugins/builtin/agent/public/run", () => ({ useIsCurrentRootRunning: () => false }));
vi.mock("@/plugins/builtin/chat/composer/public/fileMentions", () => ({
  useFileMentions: () => ({ handleKeyDown: () => false, open: false }),
}));
vi.mock("@/plugins/builtin/chat/composer/public/input", () => ({ imageFiles: () => [] }));
vi.mock("@/plugins/builtin/chat/composer/public/submit", () => ({
  submitComposer: () => submitted(),
}));
vi.mock("../application/focus", () => ({ setComposerFocusTarget: () => {} }));
vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => true,
}));
vi.mock("@/plugins/sdk", () => ({
  COMPOSER_KEY_BINDING: "composer-key-binding",
  lookupExtensionByKey: (_point: unknown, key: string) =>
    key === "enter" ? { handler: ({ submit }: { submit: () => boolean }) => submit() } : undefined,
}));

import { useComposerInputController } from "./useComposerInputController";

type Controller = ReturnType<typeof useComposerInputController>;

function mount() {
  return renderHook(() =>
    useComposerInputController({
      value: "你好",
      onChange: () => {},
      onClear: () => {},
      onSend: () => true,
      images: [],
      pastes: [],
      recordHistory: () => {},
      onAddImages: () => {},
      onAddPaste: () => {},
      acceptsImages: true,
    }),
  );
}

/** A plain Enter: no modifiers, no composition flags. What both the IME and the reader send. */
const enterKey = () =>
  ({
    nativeEvent: {
      key: "Enter",
      keyCode: 13,
      isComposing: false,
      altKey: false,
      ctrlKey: false,
      metaKey: false,
      shiftKey: false,
    },
    preventDefault: () => {},
  }) as never;

const committedText = { currentTarget: { value: "你好", selectionStart: 2 } } as never;

function drive(steps: (controller: Controller) => void): number {
  submitted.mockClear();
  const { result } = mount();
  act(() => steps(result.current));
  return submitted.mock.calls.length;
}

describe("useComposerInputController — Enter after an IME commit", () => {
  beforeEach(() => {
    // `performance` only: the controller reads the clock, and faking timers wholesale would
    // also stop the `requestAnimationFrame` a mention apply schedules.
    vi.useFakeTimers({ toFake: ["performance"] });
  });
  afterEach(() => vi.useRealTimers());

  it("sends on a plain Enter when no composition is involved", () => {
    expect(drive((c) => c.handleKeyDown(enterKey()))).toBe(1);
  });

  it("does not send on the Enter that committed the composition", () => {
    // The shape this guard exists for: some IMEs end the composition and then emit an ordinary
    // Enter from the same keypress. Sending there would post a half-written message.
    expect(
      drive((c) => {
        c.handleCompositionStart();
        c.handleCompositionEnd(committedText);
        c.handleKeyDown(enterKey());
      }),
    ).toBe(0);
  });

  it("sends on the NEXT Enter after that one", () => {
    expect(
      drive((c) => {
        c.handleCompositionStart();
        c.handleCompositionEnd(committedText);
        c.handleKeyDown(enterKey());
        c.handleKeyUp();
        c.handleKeyDown(enterKey());
      }),
    ).toBe(1);
  });

  it("sends when the composition was committed with the mouse", () => {
    // Picking a candidate from the IME panel ends the composition with no key events at all, so
    // nothing clears the pending flag and the reader's own Enter is the next key the controller
    // sees. Unbounded, that Enter was swallowed and the message took two presses.
    expect(
      drive((c) => {
        c.handleCompositionStart();
        c.handleCompositionEnd(committedText);
        vi.advanceTimersByTime(400);
        c.handleKeyDown(enterKey());
      }),
    ).toBe(1);
  });

  it("still refuses an Enter that arrives within the same input burst", () => {
    // The other side of the window: close enough to be the IME's own key, so it must not send.
    expect(
      drive((c) => {
        c.handleCompositionStart();
        c.handleCompositionEnd(committedText);
        vi.advanceTimersByTime(20);
        c.handleKeyDown(enterKey());
      }),
    ).toBe(0);
  });

  it("does not send while the composition is still open", () => {
    expect(
      drive((c) => {
        c.handleCompositionStart();
        c.handleKeyDown(enterKey());
      }),
    ).toBe(0);
  });
});
