import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { notifyError } from "@/features/notifications/public/notifications";
import { useComposerInputController } from "@/features/composer/ui/useComposerInputController";
const submitted = vi.fn(() => true);

vi.mock("@/lib/i18n/index", () => ({ useT: () => (key: string) => key }));

vi.mock("@/features/agent/public/session", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/features/agent/public/session")>()),
  useActiveSessionWorkspace: () => ({ status: "ready", cwd: "/w" }),
}));

vi.mock("@/features/agent/public/run", () => ({ useIsCurrentRootRunning: () => false }));

vi.mock("@/features/composer/application/fileMentions", () => ({
  useFileMentions: () => ({ open: false, items: [] }),
  useKnownWorkspacePaths: () => new Set(),
}));

vi.mock("@/features/composer/application/slashSuggestions", () => ({
  useSlashSuggestions: () => ({ open: false, items: [] }),
}));

vi.mock("@/features/composer/public/submit", () => ({
  submitComposer: () => submitted(),
}));

vi.mock("@/features/composer/public/focus", () => ({
  registerComposerFocusTarget: () => () => {},
  captureComposerFocus: () => () => {},
}));

vi.mock("@/features/connection/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => true,
}));

vi.mock("@/features/notifications/public/notifications", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/notifications/public/notifications")>();
  return { ...actual, notifyError: vi.fn() };
});
vi.mock("@/features/composer/public/extensionPoints", async (importOriginal) => {
  const actual =
    await importOriginal<typeof import("@/features/composer/public/extensionPoints")>();
  return { ...actual, COMPOSER_KEY_BINDING: "composer-key-binding" };
});
vi.mock("@/extensions/selectors/extensions", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/extensions/selectors/extensions")>();
  return {
    ...actual,
    lookupExtensionByKey: (_point: unknown, key: string) =>
      key === "enter"
        ? { handler: ({ submit }: { submit: () => boolean }) => submit() }
        : undefined,
  };
});

type Controller = ReturnType<typeof useComposerInputController>;

function mount(options: Partial<Parameters<typeof useComposerInputController>[0]> = {}) {
  return testClient.renderHook(() =>
    useComposerInputController({
      value: "你好",
      onChange: () => {},
      captureSubmission: () => ({ value: "你好", images: [], pastes: [], consume: () => true }),
      onSend: () => true,
      images: [],
      pastes: [],
      onAddImages: () => {},
      onAddPaste: () => {},
      acceptsImages: true,
      ...options,
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

describe("clipboard image admission", () => {
  it.each([true, false])("preserves the draft when image support is %s", (acceptsImages) => {
    vi.mocked(notifyError).mockClear();
    const onAddImages = vi.fn();
    const onChange = vi.fn();
    const { result } = mount({ acceptsImages, onAddImages, onChange });
    const image = new File(["image"], "image.png", { type: "image/png" });
    const preventDefault = vi.fn();
    act(() =>
      result.current.handlePaste({
        clipboardData: { files: [image], getData: () => "" },
        preventDefault,
      } as never),
    );
    expect(preventDefault).toHaveBeenCalledOnce();
    expect(onAddImages).toHaveBeenCalledTimes(acceptsImages ? 1 : 0);
    expect(notifyError).toHaveBeenCalledTimes(acceptsImages ? 0 : 1);
    expect(onChange).not.toHaveBeenCalled();
  });

  it("lets mixed clipboard text use native insertion when images are unsupported", () => {
    const onAddImages = vi.fn();
    const { result } = mount({ acceptsImages: false, onAddImages });
    const preventDefault = vi.fn();
    act(() =>
      result.current.handlePaste({
        clipboardData: {
          files: [new File(["image"], "a.png", { type: "image/png" })],
          getData: () => "caption",
        },
        preventDefault,
      } as never),
    );
    expect(preventDefault).not.toHaveBeenCalled();
    expect(onAddImages).not.toHaveBeenCalled();
  });
});
