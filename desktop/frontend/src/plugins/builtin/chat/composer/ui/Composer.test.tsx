import { type ReactElement, useState } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { contributeLayout, definePlugin } from "@/plugins/sdk";
import { COMPOSER_KEY_BINDING, SLASH_COMMAND } from "@/plugins/sdk/kernelPoints";
import { mountLocaleBundle, setLocale } from "@/lib/i18n";
import { WORKSPACE_LIST_FILES_KEY } from "@/plugins/builtin/workspace/public/queries";
import { Composer } from "./Composer";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { useComposerStore } from "../adapters/composerStore";
import {
  useAddComposerImageFiles,
  useAddComposerPaste,
  useComposerImages,
  useComposerPastes,
  useEditComposerPaste,
  useRemoveComposerImage,
  useRemoveComposerPaste,
  useRestoreComposerPaste,
} from "../public/attachments";
import { captureComposerSubmission, useComposerText, useSetComposerText } from "../public/draft";

vi.mock("@/plugins/builtin/runtime/public/serviceStatus", () => ({
  runtimeCommandsAvailable: () => true,
}));

function wrap(ui: ReactElement) {
  const client = new QueryClient({
    defaultOptions: {
      queries: { retry: false, gcTime: Infinity },
    },
  });
  client.setQueryData([WORKSPACE_LIST_FILES_KEY, undefined], []);
  return render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>);
}

async function withEnterKeymap() {
  await loadPluginsForTest(
    definePlugin({
      name: "test.composer-keymap",
      setup: (ctx) => {
        ctx.contribute(COMPOSER_KEY_BINDING, {
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
      pastes: baseProps.pastes,
      consume: () => true,
    }),
  };
}

afterEach(async () => {
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
          contributeLayout(ctx, "composer.header", {
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
          { id: "a1", mime: "image/png", data: "AAAA", name: "shot1.png" },
          { id: "a2", mime: "image/jpeg", data: "BBBB", name: "shot2.jpg" },
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
            ctx.contribute(COMPOSER_KEY_BINDING, {
              key: "Enter",
              description: "submit",
              handler: ({ submit }) => {
                submit();
                return true;
              },
            });
            ctx.contribute(COMPOSER_KEY_BINDING, {
              key: "Escape",
              description: "stop",
              handler: stop,
            });
            ctx.contribute(SLASH_COMMAND, { description: "goal", run: () => {} }, { key: "/goal" });
            ctx.contribute(SLASH_COMMAND, { description: "grep", run: () => {} }, { key: "/grep" });
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
      captureSubmission={captureComposerSubmission}
      onSend={() => true}
      images={useComposerImages()}
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

describe("reviewing a pasted text", () => {
  const PASTE = Array.from({ length: 18 }, (_, i) => `第 ${i + 1} 行 · mixed line ${i + 1}`).join(
    "\n",
  );
  const EDITED = "Edited evidence · 修改后正文\nFinal line";
  const IMAGE = { mime: "image/png", data: "AAAA", name: "shot.png" };

  function draft() {
    return useComposerStore.getState().composer.draft;
  }

  function stage(...pastes: string[]) {
    const store = useComposerStore.getState();
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

  afterEach(() => useComposerStore.getState().clear());

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
    const store = useComposerStore.getState();
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
      useComposerStore.getState().loadSession("other");
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
