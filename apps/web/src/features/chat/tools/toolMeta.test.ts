import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { type ToolCall } from "@/features/agent/public/sessionView";
import { TOOL_ACTION } from "@/features/chat/public/tools/types";
import { toolActions } from "@/features/chat/tools/toolMeta";
import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { activeLocale, mountLocaleBundle, setLocale } from "@/lib/i18n/index";
import { loadPluginsForTest } from "@/test/kernel";

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

const args = '  {"query":"  配置  文件  "}  ';

const tool: ToolCall = {
  id: "grep-1",
  runId: "root-run",
  name: "notes.inspect",
  category: "unknown",
  fn: "notes.inspect",
  args,
  safetyClass: "safe",
  status: "ok",
};

const disposers: Array<() => void> = [];

let writeText: ReturnType<typeof vi.fn<(text: string) => Promise<void>>>;
let previousLocale: string;

beforeEach(async () => {
  previousLocale = activeLocale();
  await loadPluginsForTest(toolActions);
  writeText = vi.fn<(text: string) => Promise<void>>();
  vi.stubGlobal("navigator", { clipboard: { writeText } });
});

afterEach(async () => {
  setLocale(previousLocale);
  await Promise.resolve();
  while (disposers.length) disposers.pop()!();
  vi.unstubAllGlobals();
  vi.mocked(toast.success).mockClear();
  vi.mocked(toast.error).mockClear();
});

async function copyArguments() {
  const action = lookupExtensionPoint(TOOL_ACTION).find(({ id }) => id === "copy-args");
  expect(action?.predicate?.(tool)).toBe(true);
  await action!.run(tool);
}

it("acknowledges arguments copied exactly", async () => {
  writeText.mockResolvedValue(undefined);

  await copyArguments();

  expect(writeText).toHaveBeenCalledExactlyOnceWith(args);
  expect(toast.success).toHaveBeenCalledExactlyOnceWith("Arguments copied");
  expect(toast.error).not.toHaveBeenCalled();
});

it("points a refused copy at the Arguments section without echoing the material", async () => {
  writeText.mockRejectedValue(new Error("NotAllowedError: secret-token"));

  await copyArguments();

  expect(writeText).toHaveBeenCalledExactlyOnceWith(args);
  expect(toast.success).not.toHaveBeenCalled();
  expect(toast.error).toHaveBeenCalledExactlyOnceWith(
    "Couldn't copy arguments. Expand the tool and copy them from Arguments.",
  );
});

it("speaks the locale active when the action runs, not when the plugin was installed", async () => {
  disposers.push(
    mountLocaleBundle("tool-action-fixture", {
      "toolAction.copyFailed": "Fixture failure from {{section}}",
      "toolInspector.arguments": "Fixture section",
    }),
  );
  setLocale("tool-action-fixture");
  await Promise.resolve();
  writeText.mockRejectedValue(new Error("denied"));

  await copyArguments();

  expect(toast.error).toHaveBeenCalledExactlyOnceWith("Fixture failure from Fixture section");
});
