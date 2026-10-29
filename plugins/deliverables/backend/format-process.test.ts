import { access, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { Refusal } from "@qimu/node";
import { Formats } from "./format-process";
import { runProcess, UnconfirmedProcessExit } from "./process";
import type { ProcessPlan } from "./confinement";
import { LIMITS } from "../contracts/limits";

vi.mock("node:fs/promises", async (original) => {
  const actual = await original<typeof import("node:fs/promises")>();
  return { ...actual, rm: vi.fn(actual.rm) };
});

vi.mock("./process", async (original) => {
  const actual = await original<typeof import("./process")>();
  return { ...actual, runProcess: vi.fn(actual.runProcess) };
});

let packaged: string;
let formats: Formats;
let started: PromiseWithResolvers<{
  plan: ProcessPlan;
  signal: AbortSignal;
}>;
let completed: PromiseWithResolvers<{ stdout: string; stderr: string }>;
const scratch: string[] = [];
const actualFS =
  await vi.importActual<typeof import("node:fs/promises")>("node:fs/promises");
let releaseCleanup: (() => void) | undefined;

beforeEach(async () => {
  vi.mocked(rm).mockReset().mockImplementation(actualFS.rm);
  packaged = await mkdtemp(join(tmpdir(), "flame-format-fixture-"));
  const entry = join(packaged, "start.mjs");
  await writeFile(entry, "export {};\n");
  started = Promise.withResolvers();
  completed = Promise.withResolvers();
  vi.mocked(runProcess)
    .mockReset()
    .mockImplementation((plan, signal) => {
      scratch.push(plan.cwd);
      started.resolve({ plan, signal });
      return completed.promise;
    });
  formats = new Formats(entry);
});

afterEach(async () => {
  releaseCleanup?.();
  releaseCleanup = undefined;
  completed.resolve({ stdout: "", stderr: "" });
  await formats.close().catch(() => {});
  vi.mocked(rm).mockImplementation(actualFS.rm);
  for (const path of [...scratch.splice(0), packaged])
    await rm(path, { recursive: true, force: true });
  vi.restoreAllMocks();
  vi.useRealTimers();
});

it("reserves its operation before asynchronous scratch preparation", async () => {
  const operation = formats.create("docx", "{}", new AbortController().signal);
  await expect(
    formats.create("docx", "{}", new AbortController().signal),
  ).rejects.toMatchObject({ code: "formats_busy" });
  const { plan } = await started.promise;
  completed.resolve({
    stdout: JSON.stringify({ bytes: Buffer.from("native").toString("base64") }),
    stderr: "",
  });
  expect(await operation).toEqual(Buffer.from("native"));
  await expect(access(plan.cwd)).rejects.toMatchObject({ code: "ENOENT" });
});

it("keeps cancellation and shutdown attached to the original process result", async () => {
  const controller = new AbortController();
  const failure = new Error("cancelled by fixture");
  const operation = formats.create("docx", "{}", controller.signal);
  const failed = expect(operation).rejects.toBe(failure);
  const { plan, signal } = await started.promise;
  controller.abort(failure);
  expect(signal.aborted).toBe(true);
  await expect(
    formats.create("docx", "{}", new AbortController().signal),
  ).rejects.toMatchObject({ code: "formats_busy" });
  let joined = false;
  const closing = formats.close().then(() => {
    joined = true;
  });
  await Promise.resolve();
  expect(joined).toBe(false);
  await access(plan.cwd);
  completed.reject(failure);
  await failed;
  await closing;
  expect(joined).toBe(true);
  await expect(access(plan.cwd)).rejects.toMatchObject({ code: "ENOENT" });
});

it("retains scratch and closes permanently when process exit is unconfirmed", async () => {
  const operation = formats.create("docx", "{}", new AbortController().signal);
  const failure = new UnconfirmedProcessExit("fixture exit is unconfirmed");
  const failed = expect(operation).rejects.toBe(failure);
  const { plan } = await started.promise;
  completed.reject(failure);
  await failed;
  await access(plan.cwd);
  await expect(formats.close()).rejects.toBe(failure);
  await expect(
    formats.create("docx", "{}", new AbortController().signal),
  ).rejects.toMatchObject({ code: "formats_closed" });
  expect(runProcess).toHaveBeenCalledTimes(1);
});

it("does not publish a worker result when its private deadline expires during cleanup", async () => {
  vi.useFakeTimers();
  const cleanupStarted = Promise.withResolvers<void>();
  const cleanupReleased = Promise.withResolvers<void>();
  releaseCleanup = () => cleanupReleased.resolve();
  vi.mocked(rm).mockImplementationOnce(async (path, options) => {
    cleanupStarted.resolve();
    await cleanupReleased.promise;
    await actualFS.rm(path, options);
  });
  const operation = formats.create("docx", "{}", new AbortController().signal);
  const failed = expect(operation).rejects.toMatchObject({
    code: "format_timeout",
  });
  const { plan } = await started.promise;
  completed.resolve({
    stdout: JSON.stringify({ bytes: Buffer.from("native").toString("base64") }),
    stderr: "",
  });
  await cleanupStarted.promise;
  await vi.advanceTimersByTimeAsync(LIMITS.formatMs);
  cleanupReleased.resolve();
  await failed;
  await expect(access(plan.cwd)).rejects.toMatchObject({ code: "ENOENT" });
});

it.each([
  { failure: { code: 1, message: "invalid failure" } },
  { failure: { code: "invalid_native", message: "invalid", extra: true } },
  { inspection: { format: "docx", items: [], facts: {}, truncated: false } },
])(
  "rejects malformed worker evidence without inventing a business refusal: %j",
  async (response) => {
    completed.resolve({ stdout: JSON.stringify(response), stderr: "" });
    await expect(
      formats.inspect(
        "docx",
        Buffer.from("native"),
        new AbortController().signal,
      ),
    ).rejects.not.toBeInstanceOf(Refusal);
    const { plan } = await started.promise;
    await expect(access(plan.cwd)).rejects.toMatchObject({ code: "ENOENT" });
  },
);

it("does not accept an inspection of a different native format", async () => {
  completed.resolve({
    stdout: JSON.stringify({
      inspection: {
        format: "pptx",
        items: [],
        facts: {},
        checks: [],
        truncated: false,
      },
    }),
    stderr: "",
  });
  await expect(
    formats.inspect(
      "docx",
      Buffer.from("native"),
      new AbortController().signal,
    ),
  ).rejects.toThrow("different source format");
});

it("preserves a bounded process refusal and releases its confirmed scratch", async () => {
  const operation = formats.create("docx", "{}", new AbortController().signal);
  const failed = expect(operation).rejects.toMatchObject({
    code: "process_output_limit",
  });
  const { plan } = await started.promise;
  completed.reject(new Refusal("process_output_limit", "fixture output limit"));
  await failed;
  await formats.close();
  await expect(access(plan.cwd)).rejects.toMatchObject({ code: "ENOENT" });
});
