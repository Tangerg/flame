import { createHash, randomUUID } from "node:crypto";
import {
  closeSync,
  existsSync,
  fsyncSync,
  linkSync,
  mkdirSync,
  openSync,
  readdirSync,
  readFileSync,
  unlinkSync,
  writeFileSync,
} from "node:fs";
import { join } from "node:path";
import type { MutationJournalStorage } from "@flame/runtime-contract/client/mutationJournal";

export class CommandStorage implements MutationJournalStorage {
  readonly #directory: string;

  constructor(directory: string, endpoint: string) {
    this.#directory = join(directory, createHash("sha256").update(endpoint).digest("hex"));
    mkdirSync(this.#directory, { recursive: true, mode: 0o700 });
  }

  keys(): string[] {
    return readdirSync(this.#directory)
      .filter((name) => !name.endsWith(".tmp"))
      .map((name) => {
        const match = /^record-([A-Za-z0-9_-]+)\.json$/.exec(name);
        if (!match)
          throw new Error(
            "unsupported IDE command storage; resolve saved commands before upgrading",
          );
        const key = new TextDecoder("utf-8", { fatal: true }).decode(
          Buffer.from(match[1]!, "base64url"),
        );
        if (this.#name(key) !== name) throw new Error("invalid IDE command storage key");
        return key;
      });
  }

  get(key: string): unknown {
    try {
      return JSON.parse(readFileSync(this.#path(key), "utf8"));
    } catch (error) {
      if (hasCode(error, "ENOENT")) return undefined;
      throw error;
    }
  }

  set(key: string, value: unknown): void {
    const path = this.#path(key);
    const temporary = `${path}.${randomUUID()}.tmp`;
    let descriptor: number | undefined;
    try {
      descriptor = openSync(temporary, "wx", 0o600);
      writeFileSync(descriptor, JSON.stringify(value));
      fsyncSync(descriptor);
      closeSync(descriptor);
      descriptor = undefined;
      // Publication is atomic and exclusive; another extension host never reads partial input.
      linkSync(temporary, path);
      this.#syncDirectory();
    } finally {
      if (descriptor !== undefined) closeSync(descriptor);
      if (existsSync(temporary)) unlinkSync(temporary);
    }
  }

  remove(key: string): void {
    try {
      unlinkSync(this.#path(key));
      this.#syncDirectory();
    } catch (error) {
      if (!hasCode(error, "ENOENT")) throw error;
    }
  }

  #syncDirectory(): void {
    if (process.platform === "win32") return;
    const descriptor = openSync(this.#directory, "r");
    try {
      fsyncSync(descriptor);
    } finally {
      closeSync(descriptor);
    }
  }

  #name(key: string): string {
    return `record-${Buffer.from(key).toString("base64url")}.json`;
  }

  #path(key: string): string {
    return join(this.#directory, this.#name(key));
  }
}

function hasCode(error: unknown, code: string): boolean {
  return error instanceof Error && "code" in error && error.code === code;
}
