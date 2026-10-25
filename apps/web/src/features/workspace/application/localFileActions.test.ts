import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { configureLocalWorkspace } from "@/features/workspace/application/ports/localWorkspace";
import { openLocalFile, revealLocalFile } from "@/features/workspace/application/localFileActions";
const { record } = vi.hoisted(() => ({ record: vi.fn() }));

vi.mock("@/features/notifications/public/notifications", () => ({
  useNotificationStore: { getState: () => ({ push: record }) },
}));

const open = vi.fn<(cwd: string, path: string) => Promise<boolean>>();
const reveal = vi.fn<(cwd: string, path: string) => Promise<boolean>>();
let dispose: () => void;

beforeEach(() => {
  record.mockReset();
  open.mockReset().mockResolvedValue(true);
  reveal.mockReset().mockResolvedValue(true);
  dispose = configureLocalWorkspace({ available: () => true, open, reveal });
});

afterEach(() => dispose());

describe.each([
  { name: "open", run: openLocalFile, host: open, other: reveal },
  { name: "reveal", run: revealLocalFile, host: reveal, other: open },
])("local file $name", ({ run, host, other }) => {
  it("delivers the original target once and stays quiet on success", async () => {
    await expect(run("/work/project/", "/work/project/文件/logo.png")).resolves.toEqual({
      ok: true,
    });

    expect(host).toHaveBeenCalledExactlyOnceWith("/work/project/", "/work/project/文件/logo.png");
    expect(other).not.toHaveBeenCalled();
    expect(record).not.toHaveBeenCalled();
  });

  it("explains unavailable local access instead of treating false as success", async () => {
    host.mockResolvedValueOnce(false);

    const result = await run("/work/project", "assets/logo.png");

    expect(record).toHaveBeenCalledOnce();
    expect(record.mock.calls[0]![0].message).toContain("assets/logo.png");
    expect(record.mock.calls[0]![0].message).toContain("Try again");
    expect(record.mock.calls[0]![0].message).toContain(
      "Local files are unavailable for the connected Runtime.",
    );
    expect(result).toEqual({ ok: false, error: record.mock.calls[0]![0].message });
  });

  it("handles a rejected native action and retains its actual cause", async () => {
    host.mockRejectedValueOnce(new Error("Permission denied"));

    await expect(run("/work/project", "assets/logo.png")).resolves.toMatchObject({ ok: false });

    expect(record).toHaveBeenCalledOnce();
    expect(record.mock.calls[0]![0].message).toContain("assets/logo.png");
    expect(record.mock.calls[0]![0].message).toContain("Permission denied");
  });
});

it("keeps the original target when another host and action replace the current context", async () => {
  const first = Promise.withResolvers<boolean>();
  reveal.mockReturnValueOnce(first.promise);
  const operation = revealLocalFile("/work/old", "src/first.go");
  const next = configureLocalWorkspace({ available: () => true, open, reveal });
  try {
    await openLocalFile("/work/new", "src/second.go");
    first.reject(new Error("Original target no longer exists"));
    await operation;

    expect(record).toHaveBeenCalledOnce();
    expect(record.mock.calls[0]![0].message).toContain("src/first.go");
    expect(record.mock.calls[0]![0].message).not.toContain("src/second.go");
    expect(record.mock.calls[0]![0].message).toContain("Original target no longer exists");
  } finally {
    next();
  }
});

it("still names the failed action and recovery when a rejection has no diagnostic text", async () => {
  open.mockRejectedValueOnce(null);

  await expect(openLocalFile("/work/project", "report.pdf")).resolves.toMatchObject({ ok: false });

  expect(record).toHaveBeenCalledOnce();
  const [{ message }] = record.mock.calls[0]!;
  expect(message).toContain("report.pdf");
  expect(message).toContain("Try again");
  expect(message).not.toContain("undefined");
});

it("handles a synchronous host refusal through the same feedback boundary", async () => {
  reveal.mockImplementationOnce(() => {
    throw new Error("Finder is unavailable");
  });

  await expect(revealLocalFile("/work/project", "src/main.go")).resolves.toMatchObject({
    ok: false,
  });

  expect(record).toHaveBeenCalledOnce();
  expect(record.mock.calls[0]![0].message).toContain("Finder is unavailable");
});
