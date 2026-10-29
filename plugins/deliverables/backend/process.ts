import { execFile, spawn } from "node:child_process";
import { performance } from "node:perf_hooks";
import { setTimeout as delay } from "node:timers/promises";
import { Refusal } from "@qimu/node";
import { LIMITS } from "../contracts/limits";
import type { ProcessPlan } from "./confinement";

export class UnconfirmedProcessExit extends Error {}

// Darwin's /bin/ps is setuid and cannot execute inside an inherited Seatbelt.
// These proc_info constants and the 96-byte proc_taskinfo layout come from
// sys/syscall.h and sys/proc_info.h; resident size is the second uint64_t.
const residentProbe = `use strict;
use warnings;
my $task_info = "\\0" x 96;
my $read = syscall(336, 2, 0 + $ARGV[0], 4, 0, $task_info, length($task_info));
exit 3 if $read < 0 && $!{ESRCH};
die "deliverables: proc_info failed: $!" unless $read == length($task_info);
print unpack("x8 Q", $task_info);
`;

interface ProcessIO {
  input?: Uint8Array;
  inputBytes?: number;
  stdoutBytes?: number;
  stderrBytes?: number;
}

function missing(cause: unknown): boolean {
  return cause instanceof Error && "code" in cause && cause.code === "ESRCH";
}

function groupExists(pid: number): boolean {
  try {
    process.kill(-pid, 0);
    return true;
  } catch (cause) {
    if (missing(cause)) return false;
    throw new UnconfirmedProcessExit(
      "deliverables: process group cannot be inspected",
      { cause },
    );
  }
}

function residentBytes(pid: number): Promise<number> {
  return new Promise((resolve, reject) => {
    execFile(
      "/usr/bin/perl",
      ["-e", residentProbe, "--", String(pid)],
      {
        encoding: "utf8",
        env: { LANG: "C", LC_ALL: "C" },
        timeout: 1000,
        killSignal: "SIGKILL",
        maxBuffer: 1024,
      },
      (error, stdout) => {
        if (error)
          return reject(
            error.code === 3 ? Object.assign(error, { code: "ESRCH" }) : error,
          );
        const value = stdout.trim();
        const bytes = Number(value);
        if (!/^\d+$/.test(value) || !Number.isSafeInteger(bytes))
          return reject(
            new Error("deliverables: invalid resident memory observation"),
          );
        resolve(bytes);
      },
    );
  });
}

// One call owns every captured byte and the actual close. Linux's PID namespace
// contains descendants; Darwin's policy denies fork, so its child handle suffices.
export async function runProcess(
  plan: ProcessPlan,
  signal: AbortSignal,
  options: ProcessIO = {},
): Promise<{ stdout: string; stderr: string }> {
  signal.throwIfAborted();
  const inputLimit = options.inputBytes ?? LIMITS.inputBytes;
  const outputLimit = options.stdoutBytes ?? LIMITS.outputBytes;
  const errorLimit = options.stderrBytes ?? LIMITS.outputBytes;
  if (
    [inputLimit, outputLimit, errorLimit].some(
      (value) => !Number.isSafeInteger(value) || value < 0,
    )
  )
    throw new RangeError("deliverables: invalid process stream bound");
  if (
    options.input !== undefined &&
    (!(options.input instanceof Uint8Array) ||
      options.input.byteLength > inputLimit)
  )
    throw new Refusal(
      "process_input_limit",
      "The confined process input exceeds its admitted byte limit.",
    );
  const input =
    options.input === undefined ? undefined : Buffer.from(options.input);
  const child = spawn(plan.command, plan.args, {
    cwd: plan.cwd,
    env: plan.env,
    detached: true,
    shell: false,
    stdio: [input === undefined ? "ignore" : "pipe", "pipe", "pipe"],
  });
  const stdout: Buffer[] = [];
  const stderr: Buffer[] = [];
  let outputSize = 0;
  let errorSize = 0;
  let failure: Error | undefined;
  let exited = false;
  let inputFinished = input === undefined;
  let exitTimer: ReturnType<typeof setTimeout> | undefined;
  const monitoring = new AbortController();
  let monitor = Promise.resolve();
  const unconfirmed = Promise.withResolvers<never>();
  const awaitExit = () => {
    exitTimer ??= setTimeout(() => {
      unconfirmed.reject(
        new UnconfirmedProcessExit(
          "deliverables: confined process exit is unconfirmed",
        ),
      );
    }, LIMITS.processExitMs);
  };
  const stop = () => {
    monitoring.abort();
    if (child.pid) {
      try {
        if (plan.containment.kind === "namespace")
          process.kill(-child.pid, "SIGKILL");
        else if (!exited) child.kill("SIGKILL");
      } catch (cause) {
        if (!missing(cause)) {
          failure = new UnconfirmedProcessExit(
            "deliverables: confined process could not be stopped",
            { cause },
          );
          unconfirmed.reject(failure);
        }
      }
    }
    awaitExit();
  };
  const capture = (target: Buffer[], chunk: Buffer, isError: boolean) => {
    if (isError) errorSize += chunk.byteLength;
    else outputSize += chunk.byteLength;
    if (outputSize > outputLimit || errorSize > errorLimit) {
      failure ??= new Refusal(
        "process_output_limit",
        "The confined process exceeded its admitted stream byte limit.",
      );
      stop();
      return;
    }
    target.push(Buffer.from(chunk));
  };
  child.stdout!.on("data", (chunk: Buffer) => capture(stdout, chunk, false));
  child.stderr!.on("data", (chunk: Buffer) => capture(stderr, chunk, true));
  for (const stream of [child.stdout!, child.stderr!])
    stream.once("error", (cause: unknown) => {
      failure ??= Object.assign(
        new Refusal(
          "process_output_failed",
          "The confined process output could not be read completely.",
        ),
        { cause },
      );
      stop();
    });
  const closed = new Promise<number | null>((resolve) => {
    child.once("error", (cause) => {
      failure ??= new Refusal(
        "process_unavailable",
        "The configured confinement tool could not be started.",
      );
      if (cause && child.pid) stop();
    });
    // Terminate descendants even when the direct command exits successfully;
    // otherwise a child retaining stdio would keep close pending indefinitely.
    child.once("exit", () => {
      exited = true;
      monitoring.abort();
      if (plan.containment.kind === "namespace") stop();
    });
    child.once("close", (status) => {
      monitoring.abort();
      resolve(status);
    });
  });
  const abort = () => stop();
  signal.addEventListener("abort", abort, { once: true });
  child.once("spawn", () => {
    if (signal.aborted) stop();
    if (plan.containment.kind === "single" && child.pid) {
      const pid = child.pid;
      const limit = plan.containment.residentLimitBytes;
      monitor = (async () => {
        try {
          while (!monitoring.signal.aborted) {
            const bytes = await residentBytes(pid);
            if (monitoring.signal.aborted) return;
            if (bytes > limit)
              throw new Refusal(
                "process_memory_limit",
                "The confined process exceeded its resident memory limit.",
              );
            await delay(LIMITS.processMemorySampleMs, undefined, {
              signal: monitoring.signal,
            });
          }
        } catch (cause) {
          if (monitoring.signal.aborted) return;
          // The kernel may observe exit before Node delivers the child event.
          // Only the original close and status can settle that operation.
          if (missing(cause)) {
            awaitExit();
            return;
          }
          failure ??=
            cause instanceof Refusal
              ? cause
              : Object.assign(
                  new Refusal(
                    "process_resources_unavailable",
                    "The confined process memory could not be measured.",
                  ),
                  { cause },
                );
          stop();
        }
      })();
    }
  });
  if (signal.aborted) stop();
  if (input !== undefined) {
    child.stdin!.once("error", (cause: unknown) => {
      if (signal.aborted || exited) return;
      failure ??= Object.assign(
        new Refusal(
          "process_input_failed",
          "The confined process did not accept its complete input.",
        ),
        { cause },
      );
      stop();
    });
    child.stdin!.once("finish", () => {
      inputFinished = true;
    });
    child.stdin!.end(input);
  }
  try {
    const status = await Promise.race([closed, unconfirmed.promise]);
    clearTimeout(exitTimer);
    if (child.pid && plan.containment.kind === "namespace") {
      const deadline = performance.now() + LIMITS.processExitMs;
      while (groupExists(child.pid)) {
        if (performance.now() >= deadline)
          throw new UnconfirmedProcessExit(
            "deliverables: descendants remain after process close",
          );
        await new Promise<void>((resolve) => setTimeout(resolve, 20));
      }
    }
    if (failure) throw failure;
    signal.throwIfAborted();
    if (!inputFinished)
      throw new Refusal(
        "process_input_failed",
        "The confined process closed before its complete input was written.",
      );
    if (status !== 0)
      throw new Refusal(
        "process_failed",
        `The confined process failed (exit ${status ?? "signal"}).`,
      );
    return {
      stdout: Buffer.concat(stdout).toString("utf8"),
      stderr: Buffer.concat(stderr).toString("utf8"),
    };
  } finally {
    monitoring.abort();
    await monitor;
    clearTimeout(exitTimer);
    signal.removeEventListener("abort", abort);
  }
}
