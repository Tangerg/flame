import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { afterEach, describe, expect, it, vi } from "vitest";
import { COMPOSER_SUBMIT_MODE } from "@/features/composer/public/extensionPoints";
import { definePlugin } from "@/extensions/definePlugin";
import { reportPluginError } from "@/extensions/errors";
import { type SlashCommandRunCtx } from "@/features/composer/public/types";
import { SLASH_COMMAND } from "@/features/composer/public/extensionPoints";
import { submitComposer, type SubmitDeps } from "@/features/composer/application/submitComposer";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";

import {
  captureComposerFocus,
  registerComposerFocusTarget,
} from "@/features/composer/public/focus";
vi.mock("@/extensions/errors", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/extensions/errors")>();
  return { ...actual, reportPluginError: vi.fn() };
});

function deps(value: string, input: Partial<SubmitDeps> = {}): SubmitDeps {
  return {
    draft: { value, images: [], pastes: [], consume: vi.fn(() => true) },
    sendInput: vi.fn(() => true),
    canSend: () => true,
    restoreFocus: () => {},
    ...input,
  };
}

async function command(run: (context: SlashCommandRunCtx) => void | Promise<void>) {
  await loadPluginsForTest(
    definePlugin({
      name: "test.submit.slash",
      setup: (ctx) => {
        ctx.contribute(SLASH_COMMAND, { description: "echo", run }, { key: "/echo" });
      },
    }),
  );
}

describe("submitComposer", () => {
  afterEach(() => {
    resetKernelForTest();
    vi.clearAllMocks();
  });

  it("ignores whitespace without consuming the draft", () => {
    const input = deps("   ");
    submitComposer(input);
    expect(input.sendInput).not.toHaveBeenCalled();
    expect(input.draft.consume).not.toHaveBeenCalled();
  });

  it("consumes plain text only after admission", () => {
    const input = deps("hello");
    submitComposer(input);
    expect(input.sendInput).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "hello" }] });
    expect(input.draft.consume).toHaveBeenCalledExactlyOnceWith("hello");
  });

  it.each(["unavailable", "refused"])("keeps the draft when input is %s", (state) => {
    const input = deps("keep this", {
      canSend: () => state !== "unavailable",
      sendInput: vi.fn(() => state !== "refused"),
    });
    submitComposer(input);
    expect(input.draft.consume).not.toHaveBeenCalled();
  });

  it("lets an execution mode accept exactly once", async () => {
    await loadPluginsForTest(
      definePlugin({
        name: "test.submit.mode",
        setup: (ctx) => {
          ctx.contribute(COMPOSER_SUBMIT_MODE, {
            id: "goal",
            matches: () => true,
            submit: (context) => {
              context.accept();
              context.accept();
              context.clear();
            },
          });
        },
      }),
    );
    const input = deps("ship it");
    submitComposer(input);
    expect(input.sendInput).not.toHaveBeenCalled();
    expect(input.draft.consume).toHaveBeenCalledExactlyOnceWith("ship it");
  });

  it("does not treat mode completion as acceptance", async () => {
    await loadPluginsForTest(
      definePlugin({
        name: "test.submit.mode",
        setup: (ctx) => {
          ctx.contribute(COMPOSER_SUBMIT_MODE, {
            id: "goal",
            matches: () => true,
            submit: async () => {},
          });
        },
      }),
    );
    const input = deps("ship it");
    submitComposer(input);
    await Promise.resolve();
    expect(input.draft.consume).not.toHaveBeenCalled();
  });

  it.each(["sync", "async"])(
    "reports a %s slash failure once and preserves unaccepted input",
    async (kind) => {
      const failure = new Error("command rejected");
      await command(() => {
        if (kind === "sync") throw failure;
        return Promise.reject(failure);
      });
      const input = deps("/echo retry");
      expect(() => submitComposer(input)).not.toThrow();
      await Promise.resolve();
      expect(reportPluginError).toHaveBeenCalledExactlyOnceWith(
        "test.submit.slash",
        "command",
        failure,
        "command: /echo",
      );
      expect(input.draft.consume).not.toHaveBeenCalled();
    },
  );

  it("does not treat a resolved command as acceptance", async () => {
    await command(async () => {});
    const input = deps("/echo hold");
    submitComposer(input);
    await Promise.resolve();
    expect(input.draft.consume).not.toHaveBeenCalled();
  });

  it("consumes an accepted slash snapshot once before its long operation completes", async () => {
    let finish!: () => void;
    await command((context) => {
      context.accept();
      context.accept();
      return new Promise<void>((resolve) => {
        finish = resolve;
      });
    });
    const input = deps("/echo long operation");
    submitComposer(input);
    expect(input.draft.consume).toHaveBeenCalledExactlyOnceWith("/echo long operation");
    finish();
    await Promise.resolve();
    expect(input.draft.consume).toHaveBeenCalledOnce();
  });

  it.each([true, false])("projects send admission %s to the slash handler", async (accepted) => {
    const observed = vi.fn();
    await command(({ send }) => {
      observed(send("forwarded"));
    });
    const input = deps("/echo forward", { sendInput: vi.fn(() => accepted) });
    submitComposer(input);
    expect(observed).toHaveBeenCalledWith(accepted);
    expect(input.sendInput).toHaveBeenCalledWith({ parts: [{ kind: "text", text: "forwarded" }] });
    expect(input.draft.consume).toHaveBeenCalledTimes(accepted ? 1 : 0);
  });

  it("records acceptance without clearing a later revision even if its text is identical", async () => {
    let accept!: () => void;
    await command((context) => {
      accept = context.accept;
    });
    const store = () => testClient.composer.store.getState();
    store().loadSession("slash-revision");
    store().setValue("/echo same");
    const draft = store().captureSubmission();
    submitComposer(deps("", { draft }));
    store().setValue("a successor");
    store().setValue("/echo same");
    accept();
    expect(store().composer.draft.value).toBe("/echo same");
    expect(store().composer.recallOlder()?.draft.value).toBe("/echo same");
  });

  it("sends an unknown slash as ordinary input", () => {
    const input = deps("/unknown args");
    submitComposer(input);
    expect(input.sendInput).toHaveBeenCalledWith({
      parts: [{ kind: "text", text: "/unknown args" }],
    });
  });

  it.each(["look at this", "   "])("folds pasted content into input %j", (value) => {
    const input = deps(value);
    input.draft = { ...input.draft, pastes: [{ id: "p1", text: "PASTED BLOB", lines: 1 }] };
    submitComposer(input);
    expect(input.sendInput).toHaveBeenCalledWith({
      parts: [{ kind: "text", text: value.trim() ? "look at this\n\nPASTED BLOB" : "PASTED BLOB" }],
    });
    expect(input.draft.consume).toHaveBeenCalledOnce();
  });
});

describe("submission feedback", () => {
  afterEach(() => {
    resetKernelForTest();
    vi.clearAllMocks();
  });

  it("keeps an accepted command consumed if its execution later fails", async () => {
    const failure = new Error("execution failed");
    await command(({ accept }) => {
      accept();
      return Promise.reject(failure);
    });
    const input = deps("/echo accepted");
    submitComposer(input);
    await Promise.resolve();
    expect(input.draft.consume).toHaveBeenCalledExactlyOnceWith("/echo accepted");
    expect(reportPluginError).toHaveBeenCalledOnce();
  });

  it("reports asynchronous mode rejection without consuming the draft", async () => {
    const failure = new Error("mode rejected");
    await loadPluginsForTest(
      definePlugin({
        name: "test.submit.mode",
        setup: (ctx) => {
          ctx.contribute(COMPOSER_SUBMIT_MODE, {
            id: "mode",
            matches: () => true,
            submit: () => Promise.reject(failure),
          });
        },
      }),
    );
    const input = deps("keep this");
    submitComposer(input);
    await Promise.resolve();
    expect(reportPluginError).toHaveBeenCalledExactlyOnceWith(
      "test.submit.mode",
      "command",
      failure,
      "composer.submitMode:mode",
    );
    expect(input.draft.consume).not.toHaveBeenCalled();
  });

  it("retains the failing command's original plugin owner after replacement", async () => {
    let reject!: (error: Error) => void;
    await command(
      () =>
        new Promise<void>((_, failed) => {
          reject = failed;
        }),
    );
    submitComposer(deps("/echo pending"));
    resetKernelForTest();
    await loadPluginsForTest(
      definePlugin({
        name: "test.replacement",
        setup: (ctx) => {
          ctx.contribute(
            SLASH_COMMAND,
            { description: "replacement", run: () => {} },
            { key: "/echo" },
          );
        },
      }),
    );
    const failure = new Error("original execution failed");
    reject(failure);
    await Promise.resolve();
    expect(reportPluginError).toHaveBeenCalledExactlyOnceWith(
      "test.submit.slash",
      "command",
      failure,
      "command: /echo",
    );
  });
});

it("does not steal focus after a delayed acceptance", async () => {
  const editor = document.createElement("textarea");
  const other = document.createElement("button");
  document.body.append(editor, other);
  const unregister = registerComposerFocusTarget(testClient.composer.access, editor);
  try {
    let accept!: () => void;
    await command((context) => {
      accept = context.accept;
    });
    editor.focus();
    const input = deps("/echo later", {
      restoreFocus: captureComposerFocus(testClient.composer.access),
    });
    submitComposer(input);
    other.focus();
    accept();
    expect(input.draft.consume).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(other);
  } finally {
    unregister();
    editor.remove();
    other.remove();
    resetKernelForTest();
  }
});
