import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

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
    vi.useFakeTimers({ toFake: ["performance"] });
  });
  afterEach(() => vi.useRealTimers());

  it("sends on a plain Enter when no composition is involved", () => {
    expect(drive((c) => c.handleKeyDown(enterKey()))).toBe(1);
  });

  it("does not send on the Enter that committed the composition", () => {
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
