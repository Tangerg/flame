import { randomUUID } from "node:crypto";
import { constants } from "node:fs";
import { open, rename, unlink } from "node:fs/promises";
import { dirname } from "node:path";

interface OwnershipRecord {
  owner: string;
  platform: NodeJS.Platform;
  pid: number | null;
  nativeTurn: NativeTurn | null;
}

export interface NativeTurn {
  sessionId: string;
  turnId: string;
  callId: string;
}

export function sameNativeTurn(left: NativeTurn, right: NativeTurn): boolean {
  return (
    left.sessionId === right.sessionId &&
    left.turnId === right.turnId &&
    left.callId === right.callId
  );
}

function validNativeTurn(value: unknown): value is NativeTurn {
  if (
    !value ||
    typeof value !== "object" ||
    Array.isArray(value) ||
    Object.keys(value).sort().join(",") !== "callId,sessionId,turnId"
  )
    return false;
  return Object.values(value).every(
    (entry) =>
      typeof entry === "string" &&
      entry.length > 0 &&
      entry.length <= 256 &&
      Array.from(entry).every((character) => {
        const code = character.codePointAt(0)!;
        return code >= 32 && code !== 127;
      }),
  );
}

function missing(cause: unknown): boolean {
  return cause instanceof Error && "code" in cause && cause.code === "ENOENT";
}

function absentProcess(cause: unknown): boolean {
  return cause instanceof Error && "code" in cause && cause.code === "ESRCH";
}

async function syncDirectory(path: string): Promise<void> {
  if (process.platform === "win32") return;
  const directory = await open(dirname(path), "r");
  try {
    await directory.sync();
  } finally {
    await directory.close();
  }
}

async function readRecord(path: string): Promise<OwnershipRecord | undefined> {
  let file;
  try {
    file = await open(
      path,
      constants.O_RDONLY |
        (constants.O_NOFOLLOW ?? 0) |
        (constants.O_NONBLOCK ?? 0),
    );
  } catch (cause) {
    if (missing(cause)) return;
    throw new Error("computer: engine ownership cannot be inspected", {
      cause,
    });
  }
  try {
    const stat = await file.stat();
    if (!stat.isFile() || stat.size > 4096)
      throw new Error("computer: invalid engine ownership record");
    const buffer = Buffer.alloc(4097);
    let length = 0;
    while (length < buffer.length) {
      const { bytesRead } = await file.read(
        buffer,
        length,
        buffer.length - length,
      );
      if (bytesRead === 0) break;
      length += bytesRead;
    }
    if (length > 4096)
      throw new Error("computer: engine ownership record exceeds capacity");
    const record = JSON.parse(
      new TextDecoder("utf-8", { fatal: true }).decode(
        buffer.subarray(0, length),
      ),
    ) as Record<string, unknown>;
    if (
      !record ||
      typeof record !== "object" ||
      Array.isArray(record) ||
      Object.keys(record).sort().join(",") !==
        "nativeTurn,owner,pid,platform" ||
      typeof record.owner !== "string" ||
      !/^[a-f0-9-]{36}$/.test(record.owner) ||
      record.platform !== process.platform ||
      (record.pid !== null &&
        (!Number.isSafeInteger(record.pid) || Number(record.pid) <= 0)) ||
      (record.nativeTurn !== null && !validNativeTurn(record.nativeTurn))
    )
      throw new Error(
        "computer: engine ownership requires manual reconciliation",
      );
    return record as unknown as OwnershipRecord;
  } catch (cause) {
    throw new Error(
      "computer: engine ownership requires manual reconciliation",
      { cause },
    );
  } finally {
    await file.close();
  }
}

export function processExists(pid: number): boolean {
  try {
    process.kill(process.platform === "win32" ? pid : -pid, 0);
    return true;
  } catch (cause) {
    if (absentProcess(cause)) return false;
    throw new Error(
      "computer: previous engine liveness cannot be established",
      { cause },
    );
  }
}

async function writeRecord(
  path: string,
  record: OwnershipRecord,
): Promise<void> {
  const file = await open(path, "wx", 0o600);
  try {
    await file.writeFile(JSON.stringify(record) + "\n", "utf8");
    await file.sync();
  } finally {
    await file.close();
  }
  await syncDirectory(path);
}

async function guarded<T>(path: string, action: () => Promise<T>): Promise<T> {
  const guard = `${path}.lock`;
  let file;
  try {
    file = await open(guard, "wx", 0o600);
  } catch (cause) {
    throw new Error(
      "computer: engine ownership admission is unresolved; inspect its lock before retrying",
      { cause },
    );
  }
  try {
    await file.writeFile(randomUUID() + "\n", "utf8");
    await file.sync();
    await syncDirectory(guard);
    return await action();
  } finally {
    await file.close();
    await unlink(guard);
    await syncDirectory(guard);
  }
}

// The durable marker prevents a replacement backend from controlling beside an orphaned engine.
export class EngineOwnership {
  readonly #path: string;
  readonly #owner: string;
  #released = false;

  private constructor(path: string, owner: string) {
    this.#path = path;
    this.#owner = owner;
  }

  static async acquire(path: string): Promise<EngineOwnership> {
    const owner = randomUUID();
    await guarded(path, async () => {
      const previous = await readRecord(path);
      if (previous) {
        if (previous.nativeTurn !== null)
          throw new Error(
            "computer: previous native turn cleanup is unconfirmed; reconcile the control target before opening another lease",
          );
        if (previous.pid === null)
          throw new Error(
            "computer: engine spawn is unconfirmed; reconcile its ownership record before retrying",
          );
        if (processExists(previous.pid))
          throw new Error(
            "computer: previous engine process group is still present; stop it before opening another control lease",
          );
        await unlink(path);
        await syncDirectory(path);
      }
      await writeRecord(path, {
        owner,
        platform: process.platform,
        pid: null,
        nativeTurn: null,
      });
    });
    return new EngineOwnership(path, owner);
  }

  async commit(pid: number): Promise<void> {
    await guarded(this.#path, async () => {
      const record = await this.#record();
      if (record.pid !== null)
        throw new Error("computer: engine ownership already has a process");
      await this.#replace({ ...record, pid });
    });
  }

  async admitNative(turn: NativeTurn): Promise<void> {
    if (!validNativeTurn(turn))
      throw new Error("computer: invalid native turn identity");
    await guarded(this.#path, async () => {
      const record = await this.#record();
      if (record.pid === null || record.nativeTurn !== null)
        throw new Error(
          "computer: native turn admission has no exclusive process owner",
        );
      await this.#replace({ ...record, nativeTurn: { ...turn } });
    });
  }

  async endNative(turn: NativeTurn): Promise<void> {
    await guarded(this.#path, async () => {
      const record = await this.#record();
      if (
        record.nativeTurn === null ||
        !sameNativeTurn(record.nativeTurn, turn)
      )
        throw new Error(
          "computer: native cleanup does not match its original turn",
        );
      await this.#replace({ ...record, nativeTurn: null });
    });
  }

  async #replace(record: OwnershipRecord): Promise<void> {
    const temporary = `${this.#path}.${this.#owner}.tmp`;
    await writeRecord(temporary, record);
    await rename(temporary, this.#path);
    await syncDirectory(this.#path);
  }

  async #record(): Promise<OwnershipRecord> {
    const record = await readRecord(this.#path);
    if (!record || record.owner !== this.#owner)
      throw new Error("computer: engine ownership changed outside its owner");
    return record;
  }

  async release(pid?: number): Promise<void> {
    if (this.#released) return;
    await guarded(this.#path, async () => {
      const record = await this.#record();
      if (record.pid !== null && record.pid !== pid)
        throw new Error(
          "computer: engine process differs from its ownership record",
        );
      if (record.nativeTurn !== null)
        throw new Error(
          "computer: native turn cleanup is unconfirmed; ownership was retained",
        );
      if (pid !== undefined && processExists(pid))
        throw new Error(
          "computer: engine process group exit is unconfirmed; ownership was retained",
        );
      await unlink(this.#path);
      await syncDirectory(this.#path);
      this.#released = true;
    });
  }
}
