import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { type ReactElement, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { contributeLayout } from "@/extensions/contributeHelpers";
import { definePlugin } from "@/extensions/definePlugin";
import { COMPOSER_KEY_BINDING, SLASH_COMMAND } from "@/features/composer/public/extensionPoints";
import { mountLocaleBundle, setLocale, t } from "@/lib/i18n/index";
import { WORKSPACE_LIST_FILES_KEY } from "@/features/workspace/public/queries";
import { Composer } from "@/features/composer/ui/Composer";
import * as imageInput from "@/features/composer/public/input";
import { loadPluginsForTest } from "@/test/kernel";
import { mockLocalStorage } from "@/test/localStorage";

import {
  useAddComposerImageFiles,
  useAddComposerPaste,
  useComposerImageAttachments,
  useComposerPastes,
  useEditComposerPaste,
  useRemoveComposerImage,
  useRemoveComposerPaste,
  useRestoreComposerPaste,
} from "@/features/composer/public/attachments";
import {
  captureComposerSubmission,
  useComposerText,
  useSetComposerText,
} from "@/features/composer/public/draft";
vi.mock("@/features/connection/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => true,
}));

function wrap(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
    },
  });
  client.setQueryData([WORKSPACE_LIST_FILES_KEY, undefined], []);
  return testClient.render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

async function withEnterKeymap() {
  await loadPluginsForTest(
    definePlugin({
      name: "test.composer-keymap",
      setup: (ctx) => {
        ctx.contributions.add(COMPOSER_KEY_BINDING, {
          key: "Enter",
          description: "submit",
          handler: ({ submit }) => {
            submit();
            return true;
          },
        });
      },
    }),
  );
}

const baseProps = {
  images: [],
  onRemoveImage: () => {},
  onAddImages: () => {},
  pastes: [],
  onRemovePaste: () => {},
  onEditPaste: () => true,
  onRestorePaste: () => {},
  onAddPaste: () => {},
  acceptsImages: true,
  mode: "agent" as const,
  onModeChange: () => {},
};

function composerProps(value: string) {
  return {
    ...baseProps,
    value,
    captureSubmission: () => ({
      value,
      images: baseProps.images,
      attachmentsReady: true,
      pastes: baseProps.pastes,
      consume: () => true,
    }),
  };
}

afterEach(async () => {
  vi.restoreAllMocks();
  await act(async () => {
    setLocale("en");
    await Promise.resolve();
  });
});

describe("composer", () => {
  it("does not draw an internal divider for a standing surface that renders nothing", async () => {
    await loadPluginsForTest(
      definePlugin({
        name: "test.empty-composer-standing-surface",
        setup: (ctx) => {
          contributeLayout(ctx.contributions, "composer.header", {
            id: "empty",
            component: () => null,
          });
        },
      }),
    );

    const { container } = wrap(
      <Composer {...composerProps("")} onChange={() => {}} onSend={() => true} />,
    );

    expect(container.querySelector(".border-b")).toBeNull();
  });

  it("calls onChange as the user types", () => {
    const onChange = vi.fn();
    wrap(<Composer {...composerProps("")} onChange={onChange} onSend={() => true} />);
    const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
    fireEvent.change(textarea, { target: { value: "hi" } });
    expect(onChange).toHaveBeenCalledWith("hi");
  });

  it("does not reference an unmounted mention listbox", () => {
    wrap(<Composer {...composerProps("")} onChange={() => {}} onSend={() => true} />);

    expect(screen.getByRole("textbox").hasAttribute("aria-controls")).toBe(false);
  });

  it("submits non-empty text on Enter when a binding maps Enter → submit", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    const onChange = vi.fn();
    wrap(<Composer {...composerProps("hello world")} onChange={onChange} onSend={onSend} />);
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    expect(onSend).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "hello world" }] });
  });

  it("keeps the Enter that immediately follows a WebKit IME composition end inside the composer", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    wrap(<Composer {...composerProps("english")} onChange={() => {}} onSend={onSend} />);
    const textarea = screen.getByRole("textbox");

    fireEvent.compositionStart(textarea, { data: "englis" });
    fireEvent.compositionEnd(textarea, { data: "english" });
    fireEvent.keyDown(textarea, { key: "Enter", keyCode: 229, isComposing: false });

    expect(onSend).not.toHaveBeenCalled();
  });

  it("keeps an unmarked Enter that commits Latin text through a Chinese IME inside the composer", async () => {
    // Renderer work must not consume this test's immediate input-commit window.
    const clock = vi.spyOn(performance, "now").mockReturnValue(0);
    try {
      await withEnterKeymap();
      const onSend = vi.fn(() => true);
      wrap(<Composer {...composerProps("中文 english")} onChange={() => {}} onSend={onSend} />);
      const textarea = screen.getByRole("textbox");

      fireEvent.compositionStart(textarea, { data: "english" });
      fireEvent.compositionEnd(textarea, { data: "english" });
      fireEvent.keyDown(textarea, { key: "Enter", keyCode: 13, isComposing: false });

      expect(onSend).not.toHaveBeenCalled();

      await act(async () => Promise.resolve());
      fireEvent.keyDown(textarea, { key: "Enter", keyCode: 13, isComposing: false });
      expect(onSend).toHaveBeenCalledOnce();
    } finally {
      clock.mockRestore();
    }
  });

  it("recovers the same commit ownership when a browser drops compositionend", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    wrap(<Composer {...composerProps("中文 english")} onChange={() => {}} onSend={onSend} />);
    const textarea = screen.getByRole("textbox");

    fireEvent.compositionStart(textarea, { data: "english" });
    fireEvent.change(textarea, { target: { value: "中文 english" } });
    fireEvent.keyDown(textarea, { key: "Enter", keyCode: 13, isComposing: false });

    expect(onSend).not.toHaveBeenCalled();
  });

  it("does not submit when the textarea is empty / whitespace only", async () => {
    await withEnterKeymap();
    const onSend = vi.fn(() => true);
    wrap(<Composer {...composerProps("   ")} onChange={() => {}} onSend={onSend} />);
    fireEvent.keyDown(screen.getByRole("textbox"), { key: "Enter" });
    expect(onSend).not.toHaveBeenCalled();
  });

  it("renders image thumbnails when images are staged", () => {
    wrap(
      <Composer
        {...composerProps("")}
        onChange={() => {}}
        onSend={() => true}
        images={[
          { status: "ready", id: "a1", mime: "image/png", data: "AAAA", name: "shot1.png" },
          { status: "ready", id: "a2", mime: "image/jpeg", data: "BBBB", name: "shot2.jpg" },
        ]}
      />,
    );
    const imgs = screen.getAllByRole("img");
    expect(imgs).toHaveLength(2);
    expect(imgs[0]!.getAttribute("src")).toBe("data:image/png;base64,AAAA");
  });

  it("refreshes the placeholder when the locale changes", async () => {
    const unmount = mountLocaleBundle("placeholder-test", {
      "composer.input.label": "Prompt",
      "composer.placeholder": "Localized placeholder",
    });
    wrap(<Composer {...composerProps("")} onChange={() => {}} onSend={() => true} />);

    await act(async () => {
      setLocale("placeholder-test");
      await Promise.resolve();
    });

    await waitFor(() => {
      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      expect(textarea.placeholder).toBe("Localized placeholder");
    });
    unmount();
  });

  describe("slash suggestions own the keys while open", () => {
    const stop = vi.fn(() => true);

    async function withSlashCommands() {
      await loadPluginsForTest(
        definePlugin({
          name: "test.slash-keys",
          setup: (ctx) => {
            ctx.contributions.add(COMPOSER_KEY_BINDING, {
              key: "Enter",
              description: "submit",
              handler: ({ submit }) => {
                submit();
                return true;
              },
            });
            ctx.contributions.add(COMPOSER_KEY_BINDING, {
              key: "Escape",
              description: "stop",
              handler: stop,
            });
            ctx.contributions.add(
              SLASH_COMMAND,
              { description: "goal", run: () => {} },
              { key: "/goal" },
            );
            ctx.contributions.add(
              SLASH_COMMAND,
              { description: "grep", run: () => {} },
              { key: "/grep" },
            );
          },
        }),
      );
    }

    function Harness({ initial, onSend }: { initial: string; onSend: () => boolean }) {
      const [value, setValue] = useState(initial);
      return <Composer {...composerProps(value)} onChange={setValue} onSend={onSend} />;
    }

    function typeInto(value: string) {
      const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
      fireEvent.change(textarea, { target: { value, selectionStart: value.length } });
      return textarea;
    }

    it("completes a partial command on Enter instead of sending the prefix", async () => {
      await withSlashCommands();
      const onSend = vi.fn(() => true);
      wrap(<Harness initial="" onSend={onSend} />);
      const textarea = typeInto("/go");
      await screen.findByRole("option", { name: /\/goal/ });

      fireEvent.keyDown(textarea, { key: "Enter" });
      expect(onSend).not.toHaveBeenCalled();
      await waitFor(() => expect(textarea.value).toBe("/goal "));
    });

    it("closes the list on Escape without stopping the run", async () => {
      await withSlashCommands();
      stop.mockClear();
      wrap(<Harness initial="" onSend={() => true} />);
      const textarea = typeInto("/g");
      await screen.findByRole("listbox", { name: "Commands" });

      fireEvent.keyDown(textarea, { key: "Escape" });
      expect(stop).not.toHaveBeenCalled();
      await waitFor(() => expect(screen.queryByRole("listbox", { name: "Commands" })).toBeNull());

      fireEvent.keyDown(textarea, { key: "Escape" });
      expect(stop).toHaveBeenCalledOnce();
    });

    it("moves the highlight with the arrows and accepts it with Tab", async () => {
      await withSlashCommands();
      wrap(<Harness initial="" onSend={() => true} />);
      const textarea = typeInto("/g");
      await screen.findByRole("listbox", { name: "Commands" });

      fireEvent.keyDown(textarea, { key: "ArrowDown" });
      expect(textarea.getAttribute("aria-activedescendant")).toBe("composer-suggestion-option-1");
      fireEvent.keyDown(textarea, { key: "Tab" });
      await waitFor(() => expect(textarea.value).toBe("/grep "));
    });
  });
});

function DraftComposer() {
  return (
    <Composer
      value={useComposerText()}
      onChange={useSetComposerText()}
      captureSubmission={() => captureComposerSubmission(testClient.composer.access)}
      onSend={() => true}
      images={useComposerImageAttachments()}
      onRemoveImage={useRemoveComposerImage()}
      onAddImages={useAddComposerImageFiles()}
      pastes={useComposerPastes()}
      onRemovePaste={useRemoveComposerPaste()}
      onEditPaste={useEditComposerPaste()}
      onRestorePaste={useRestoreComposerPaste()}
      onAddPaste={useAddComposerPaste()}
      acceptsImages
    />
  );
}

describe("draft persistence recovery", () => {
  function refuseDraftWrites() {
    const replacement = mockLocalStorage();
    replacement.storage.setItem.mockImplementation((key, value) => {
      if (key.startsWith("flame.composer:")) {
        throw new DOMException("storage quota exceeded", "QuotaExceededError");
      }
      replacement.backing.setItem(key, value);
    });
    return replacement;
  }

  it("keeps the same input and middle caret through failure and a keyboard retry", async () => {
    act(() => testClient.composer.store.getState().setValue("  Keep this draft  "));
    wrap(<DraftComposer />);
    const input = screen.getByRole<HTMLTextAreaElement>("textbox");
    input.focus();
    const refusal = refuseDraftWrites();
    fireEvent.change(input, {
      target: { value: "  Keep edited this draft  ", selectionStart: 14, selectionEnd: 14 },
    });

    expect(screen.getByRole("alert").textContent).toContain(t("composer.persistence.saveFailed"));
    expect(screen.getByRole("textbox")).toBe(input);
    expect(document.activeElement).toBe(input);
    expect(input.value).toBe("  Keep edited this draft  ");
    expect(input.selectionStart).toBe(14);
    refusal.restore();
    const retry = screen.getByRole<HTMLButtonElement>("button", { name: "Retry" });
    retry.focus();
    fireEvent.click(retry);

    await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
    expect(screen.getByRole("textbox")).toBe(input);
    expect(document.activeElement).toBe(input);
    expect(input.selectionStart).toBe(14);
    expect(input.selectionEnd).toBe(14);
    expect(input.value).toBe("  Keep edited this draft  ");
  });

  it("keeps failed retry visible and respects a later focus move when storage recovers", () => {
    wrap(
      <>
        <DraftComposer />
        <button type="button">Elsewhere</button>
      </>,
    );
    const refusal = refuseDraftWrites();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "retain the latest draft" } });
    const retry = screen.getByRole<HTMLButtonElement>("button", { name: "Retry" });
    retry.focus();
    fireEvent.click(retry);
    expect(screen.getByRole("alert")).toBeTruthy();
    expect(document.activeElement).toBe(retry);
    const elsewhere = screen.getByRole<HTMLButtonElement>("button", { name: "Elsewhere" });
    elsewhere.focus();
    refusal.restore();
    act(() => testClient.composer.access.retryPersistence());
    expect(screen.queryByRole("alert")).toBeNull();
    expect(document.activeElement).toBe(elsewhere);
  });

  it("does not hand a retiring retry's focus to a successor input registration", () => {
    wrap(<DraftComposer />);
    const refusal = refuseDraftWrites();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "original input draft" } });
    const retry = screen.getByRole<HTMLButtonElement>("button", { name: "Retry" });
    retry.focus();
    const successor = document.createElement("textarea");
    document.body.append(successor);
    const unregister = testClient.composer.access.registerFocusTarget(successor);
    try {
      refusal.restore();
      fireEvent.click(retry);
      expect(screen.queryByRole("alert")).toBeNull();
      expect(document.activeElement).not.toBe(successor);
    } finally {
      unregister();
      successor.remove();
    }
  });

  it("copies a recovery snapshot only on request and reports clipboard refusal", async () => {
    const write = vi
      .spyOn(navigator.clipboard, "writeText")
      .mockRejectedValueOnce(new Error("denied"));
    const recovery = vi.spyOn(testClient.composer.access, "getRecoveryDrafts");
    wrap(<DraftComposer />);
    refuseDraftWrites();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "  keep exact text  " } });
    act(() => testClient.composer.store.getState().addPaste("attached\ntext"));
    expect(recovery).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole("button", { name: "Copy draft text" }));
    await screen.findByText(t("composer.persistence.copyFailed"));
    expect(recovery).toHaveBeenCalledOnce();
    expect(screen.getByRole("textbox").getAttribute("aria-label")).toBe("Message composer");
    write.mockResolvedValue(undefined);
    fireEvent.click(screen.getByRole("button", { name: "Copy draft text" }));
    await screen.findByRole("button", { name: "Draft text copied" });
    const copied = JSON.parse(write.mock.calls.at(-1)![0]) as {
      targets: Array<{
        drafts: Record<string, { value: string; pastes: Array<{ text: string }> }>;
      }>;
    };
    expect(copied.targets.flatMap((target) => Object.values(target.drafts))).toContainEqual({
      value: "  keep exact text  ",
      pastes: [expect.objectContaining({ text: "attached\ntext" })],
    });
    expect(screen.getByRole("alert").textContent).toContain(t("composer.persistence.saveFailed"));
    expect(screen.queryByText(t("composer.persistence.copyFailed"))).toBeNull();
  });

  it("keeps unread saved drafts intact and offers recovery after the user continues editing", () => {
    const target = "https://draft-read-recovery.test";
    const key = `flame.composer:${encodeURIComponent(target)}`;
    const saved = JSON.stringify({
      state: { drafts: { "": { value: "original confirmed draft", pastes: [] } } },
      version: 0,
    });
    localStorage.setItem(key, saved);
    const refusal = mockLocalStorage();
    refusal.storage.getItem.mockImplementation((name) => {
      if (name === key) throw new DOMException("storage unavailable", "SecurityError");
      return refusal.backing.getItem(name);
    });
    testClient.composer.activate(target);
    wrap(<DraftComposer />);
    expect(screen.getByRole("alert").textContent).toContain(t("composer.persistence.readFailed"));
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "keep this new draft" } });
    refusal.restore();

    expect(screen.getByRole("alert").textContent).toContain(
      t("composer.persistence.recoveryRequired"),
    );
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.getByRole("button", { name: "Copy draft text" })).toBeTruthy();
    expect(screen.getByRole<HTMLTextAreaElement>("textbox").value).toBe("keep this new draft");
    expect(localStorage.getItem(key)).toBe(saved);
    localStorage.removeItem(key);
  });

  it("does not report a late copy result as confirmation for a newer draft", async () => {
    const copying = Promise.withResolvers<void>();
    vi.spyOn(navigator.clipboard, "writeText").mockReturnValue(copying.promise);
    wrap(<DraftComposer />);
    refuseDraftWrites();
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "first unsaved draft" } });
    fireEvent.click(screen.getByRole("button", { name: "Copy draft text" }));
    fireEvent.change(input, { target: { value: "newer unsaved draft" } });
    await act(async () => {
      copying.resolve();
      await copying.promise;
    });
    expect(screen.queryByRole("button", { name: "Draft text copied" })).toBeNull();
    expect(screen.getByRole("button", { name: "Copy draft text" })).toBeTruthy();
    expect(screen.getByRole<HTMLTextAreaElement>("textbox").value).toBe("newer unsaved draft");
  });

  it("does not revive copy confirmation after another target adds an unconfirmed draft", async () => {
    vi.spyOn(navigator.clipboard, "writeText").mockResolvedValue(undefined);
    testClient.composer.activate("https://draft-copy-a.test");
    wrap(<DraftComposer />);
    refuseDraftWrites();
    fireEvent.change(screen.getByRole("textbox"), { target: { value: "unconfirmed A" } });
    fireEvent.click(screen.getByRole("button", { name: "Copy draft text" }));
    await screen.findByRole("button", { name: "Draft text copied" });
    act(() => {
      testClient.composer.activate("https://draft-copy-b.test");
      testClient.composer.store.getState().setValue("unconfirmed B");
    });
    act(() => testClient.composer.activate("https://draft-copy-a.test"));

    expect(screen.getByRole<HTMLTextAreaElement>("textbox").value).toBe("unconfirmed A");
    expect(screen.queryByRole("button", { name: "Draft text copied" })).toBeNull();
    expect(screen.getByRole("button", { name: "Copy draft text" })).toBeTruthy();
  });
});

it("announces pending and failed image input and returns focus after removal", async () => {
  const reading = Promise.withResolvers<Awaited<ReturnType<typeof imageInput.fileToInputImage>>>();
  vi.spyOn(imageInput, "fileToInputImage").mockReturnValue(reading.promise);
  await withEnterKeymap();
  const { container } = wrap(<DraftComposer />);
  const composerRoot = container.querySelector('[data-slot="composer-root"]') as HTMLElement;
  const live = within(composerRoot).getByRole("status");
  expect(live.textContent).toBe("");
  act(() => {
    testClient.composer.store.getState().setValue("keep the draft");
    testClient.composer.store.getState().addImageFiles([new File([""], "selected.png")]);
  });
  const input = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Message composer" });
  expect(live.textContent).toBe("Reading selected.png…");
  fireEvent.keyDown(input, { key: "Enter" });
  expect(input.value).toBe("keep the draft");
  await act(async () => {
    reading.reject(new Error("file read failed"));
    await reading.promise.catch(() => {});
  });
  expect(within(composerRoot).getByRole("status")).toBe(live);
  expect(live.textContent).toContain("Couldn't read selected.png");
  const remove = screen.getByRole("button", { name: "Remove image" });
  remove.focus();
  fireEvent.click(remove);
  expect(document.activeElement).toBe(input);
  expect(live.textContent).toBe("");
  expect(input.value).toBe("keep the draft");
});

describe("reviewing a pasted text", () => {
  const PASTE = Array.from({ length: 18 }, (_, i) => `第 ${i + 1} 行 · mixed line ${i + 1}`).join(
    "\n",
  );
  const EDITED = "Edited evidence · 修改后正文\nFinal line";
  const IMAGE = { mime: "image/png", data: "AAAA", name: "shot.png" };

  function draft() {
    return testClient.composer.store.getState().composer.draft;
  }

  function stage(...pastes: string[]) {
    const store = testClient.composer.store.getState();
    store.clear();
    store.setValue("Typed prefix");
    store.addImages([IMAGE]);
    for (const paste of pastes) store.addPaste(paste);
    wrap(
      <>
        <DraftComposer />
        <button type="button">Elsewhere</button>
      </>,
    );
    return screen.getByRole<HTMLTextAreaElement>("textbox");
  }

  async function review(index = 0) {
    const chip = screen.getAllByRole("button", { name: "Review pasted text" })[index]!;
    chip.focus();
    fireEvent.click(chip);
    await screen.findByRole("dialog", { name: "Pasted text" });
    const editor = screen.getByRole<HTMLTextAreaElement>("textbox", { name: "Pasted text" });
    await waitFor(() => expect(document.activeElement).toBe(editor));
    return { chip, editor };
  }

  async function closed() {
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  }

  afterEach(() => testClient.composer.store.getState().clear());

  it("puts the edited text back and lets typing continue after it", async () => {
    const input = stage(PASTE, "second paste\nkept");
    const kept = draft().pastes[1]!;
    const { editor } = await review();
    fireEvent.change(editor, { target: { value: EDITED } });
    fireEvent.click(screen.getByRole("button", { name: "Put back in message" }));
    await closed();

    expect(draft().value).toBe(`Typed prefix\n\n${EDITED}`);
    expect(draft().pastes).toEqual([kept]);
    expect(draft().images).toMatchObject([IMAGE]);
    await waitFor(() => expect(document.activeElement).toBe(input));
    expect(input.value).toBe(`Typed prefix\n\n${EDITED}`);
    expect(input.selectionStart).toBe(input.value.length);
    expect(input.selectionEnd).toBe(input.value.length);
  });

  it("returns to the composer when the last attachment is put back", async () => {
    const store = testClient.composer.store.getState();
    store.clear();
    store.setValue("Typed prefix");
    store.addPaste(PASTE);
    wrap(<DraftComposer />);
    const input = screen.getByRole<HTMLTextAreaElement>("textbox");
    await review();
    fireEvent.click(screen.getByRole("button", { name: "Put back in message" }));
    await closed();

    expect(draft().value).toBe(`Typed prefix\n\n${PASTE}`);
    expect(draft().pastes).toEqual([]);
    await waitFor(() => expect(document.activeElement).toBe(input));
    expect(input.selectionStart).toBe(input.value.length);
  });

  it("does not resurrect the original paste when an emptied review is put back", async () => {
    const input = stage(PASTE);
    const { editor } = await review();
    fireEvent.change(editor, { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Put back in message" }));
    await closed();

    expect(draft().value).toBe("Typed prefix");
    expect(draft().pastes).toEqual([]);
    expect(draft().images).toMatchObject([IMAGE]);
    await waitFor(() => expect(document.activeElement).toBe(input));
  });

  it("discards review edits on Cancel and Escape and returns to the chip", async () => {
    stage(PASTE);
    const original = draft();
    let { chip, editor } = await review();
    fireEvent.change(editor, { target: { value: EDITED } });
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await closed();
    await waitFor(() => expect(document.activeElement).toBe(chip));
    expect(draft()).toBe(original);

    ({ chip, editor } = await review());
    expect(editor.value).toBe(PASTE);
    fireEvent.change(editor, { target: { value: EDITED } });
    fireEvent.keyDown(editor, { key: "Escape" });
    await closed();
    await waitFor(() => expect(document.activeElement).toBe(chip));
    expect(draft()).toBe(original);
  });

  it("saves an edit into the chip and returns to it", async () => {
    stage(PASTE);
    const { chip, editor } = await review();
    fireEvent.change(editor, { target: { value: EDITED } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await closed();

    expect(draft().value).toBe("Typed prefix");
    expect(draft().pastes).toMatchObject([{ text: EDITED, lines: 2 }]);
    await waitFor(() => expect(document.activeElement).toBe(chip));
  });

  it("returns to the composer when saving a blank review removes the chip", async () => {
    const input = stage(PASTE, "second paste\nkept");
    const kept = draft().pastes[1]!;
    const { editor } = await review();
    fireEvent.change(editor, { target: { value: "  " } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await closed();

    expect(draft().value).toBe("Typed prefix");
    expect(draft().pastes).toEqual([kept]);
    await waitFor(() => expect(document.activeElement).toBe(input));
  });

  it("returns to the same input with its selection when one of several pastes is removed", () => {
    const input = stage(PASTE, "second paste\nkept");
    const kept = draft().pastes[1]!;
    const images = draft().images;
    input.setSelectionRange(2, 7);
    const remove = screen.getAllByRole("button", { name: t("composer.paste.remove") })[0]!;
    remove.focus();
    fireEvent.click(remove);

    expect(document.activeElement).toBe(input);
    expect(screen.getByRole("textbox")).toBe(input);
    expect(draft().value).toBe("Typed prefix");
    expect(draft().pastes).toEqual([kept]);
    expect(draft().images).toEqual(images);
    expect(input.value).toBe("Typed prefix");
    expect(input.selectionStart).toBe(2);
    expect(input.selectionEnd).toBe(7);
  });

  it("returns to the same input when removing the last paste retires the tray", () => {
    const store = testClient.composer.store.getState();
    store.clear();
    store.setValue("Typed prefix");
    store.addPaste(PASTE);
    const { container } = wrap(<DraftComposer />);
    const input = screen.getByRole<HTMLTextAreaElement>("textbox");
    const remove = screen.getByRole("button", { name: t("composer.paste.remove") });
    remove.focus();
    fireEvent.click(remove);

    expect(container.querySelector('[data-slot="composer-attachments"]')).toBeNull();
    expect(document.activeElement).toBe(input);
    expect(screen.getByRole("textbox")).toBe(input);
    expect(draft().value).toBe("Typed prefix");
    expect(draft().pastes).toEqual([]);
    expect(draft().images).toEqual([]);
  });

  it.each(["Cancel", "Save", "Put back in message"])(
    "keeps focus the user moved elsewhere while %s closes the review",
    async (action) => {
      stage(PASTE, "second paste\nkept");
      const elsewhere = screen.getByRole("button", { name: "Elsewhere" });
      await review();
      fireEvent.click(screen.getByRole("button", { name: action }));
      expect(screen.queryByRole("dialog")).not.toBeNull();
      elsewhere.focus();
      await closed();
      await act(async () => {});

      expect(document.activeElement).toBe(elsewhere);
    },
  );

  it("does not pull focus back when the Session retires an open review", async () => {
    stage(PASTE);
    const elsewhere = screen.getByRole("button", { name: "Elsewhere" });
    await review();
    act(() => {
      elsewhere.focus();
      testClient.composer.store.getState().loadSession("other");
    });
    await closed();
    await act(async () => {});

    expect(document.activeElement).toBe(elsewhere);
  });

  it("does not focus a composer that left while the review closed", async () => {
    stage(PASTE, "second paste\nkept");
    await review();
    fireEvent.click(screen.getByRole("button", { name: "Put back in message" }));
    cleanup();
    await act(async () => {});

    expect(document.activeElement).toBe(document.body);
  });
});
