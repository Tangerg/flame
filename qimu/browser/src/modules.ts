import {
  QimuError,
  type Awaitable,
  type Cleanup,
  type Lifetime,
  type Release,
  type Unit,
} from "@qimu/sdk";
import type { LocalPlan, PlannedUnit } from "@qimu/sdk/protocol";
import { sameAuthority } from "./control";

export interface ModuleLease {
  readonly url: string;
  readonly digest: string;
  readonly closed?: Promise<void>;
  close(released: Cleanup): Awaitable<void | Release>;
}

interface Evaluation {
  readonly digest: string;
  readonly promise: Promise<Record<string, unknown>>;
  leases: number;
  retired: boolean;
}

/** One instance belongs to the actual Realm, independently of individual plans. */
export class RealmModules {
  readonly #open: (
    unit: PlannedUnit,
    signal: AbortSignal,
  ) => Promise<ModuleLease>;
  readonly #installations = new Map<
    string,
    { release: string; modules: Map<string, Evaluation> }
  >();
  #authority?: LocalPlan["authority"];

  constructor(
    open: (unit: PlannedUnit, signal: AbortSignal) => Promise<ModuleLease>,
  ) {
    this.#open = open;
  }

  async evaluate(
    unit: PlannedUnit,
    owner: Lifetime,
    plan: LocalPlan,
  ): Promise<Readonly<Record<string, unknown>>> {
    owner.assertOpen();
    this.#lookup(unit, plan.authority);
    let lease: ModuleLease | undefined;
    let evaluation: Evaluation | undefined;
    const retire = () => {
      if (!lease) return;
      lease = undefined;
      if (evaluation) {
        evaluation.leases--;
        if (!evaluation.leases) evaluation.retired = true;
      }
    };
    const cleanup = owner.cleanup(async () => {
      if (!lease) return;
      const result = await lease.close({
        confirmReleased(evidence) {
          cleanup.confirmReleased(evidence);
          retire();
        },
      });
      if (result === undefined || result.status === "released") {
        retire();
      }
      return result;
    }, `module:${unit.module.path}`);
    await owner.run(async (signal) => {
      lease = await this.#open(unit, signal);
      const captured = lease;
      if (captured.closed) {
        const ended = () => {
          if (lease === captured) owner.beginClose();
        };
        void captured.closed.then(ended, ended);
      }
    });
    owner.assertOpen();
    if (lease!.digest !== unit.module.digest)
      throw new QimuError(
        "contract_mismatch",
        "leased module does not match the approved artifact digest",
      );
    const url = new URL(lease!.url);
    if (!["https:", "http:", "file:", "data:"].includes(url.protocol))
      throw new QimuError(
        "permission_denied",
        "module lease uses an unsupported URL scheme",
      );
    evaluation = this.#lookup(unit, plan.authority);
    if (!evaluation) {
      // The browser owns ESM evaluation. An attempted evaluation is never made
      // retryable by deleting a Promise or appending a cache-busting query.
      const promise = import(/* @vite-ignore */ url.href) as Promise<
        Record<string, unknown>
      >;
      this.#authority = plan.authority;
      const source = unit.instance.source;
      let installation = this.#installations.get(source.installationId);
      if (!installation) {
        installation = { release: source.release, modules: new Map() };
        this.#installations.set(source.installationId, installation);
      }
      evaluation = {
        digest: unit.module.digest,
        promise,
        leases: 0,
        retired: false,
      };
      installation.modules.set(unit.module.path, evaluation);
    }
    evaluation.leases++;
    let exports: Record<string, unknown>;
    try {
      exports = await observeEvaluation(evaluation.promise, owner.signal);
    } catch (cause) {
      owner.signal.throwIfAborted();
      throw new QimuError(
        "reload_required",
        "module evaluation failed; the realm must be replaced",
        { cause },
      );
    }
    owner.assertOpen();
    return exports;
  }

  #lookup(
    unit: PlannedUnit,
    authority: LocalPlan["authority"],
  ): Evaluation | undefined {
    const source = unit.instance.source;
    const installation = this.#installations.get(source.installationId);
    if (
      (this.#authority && !sameAuthority(this.#authority, authority)) ||
      (installation && installation.release !== source.release)
    )
      throw new QimuError(
        "reload_required",
        "installation or authority changed after module evaluation",
      );
    const evaluation = installation?.modules.get(unit.module.path);
    if (evaluation?.retired)
      throw new QimuError(
        "reload_required",
        "retired module requires a new realm",
      );
    if (evaluation && evaluation.digest !== unit.module.digest)
      throw new QimuError(
        "contract_mismatch",
        "module digest changed within an immutable release",
      );
    return evaluation;
  }

  async load(
    unit: PlannedUnit,
    owner: Lifetime,
    plan: LocalPlan,
  ): Promise<Unit> {
    const exports = await this.evaluate(unit, owner, plan);
    const definition = exports.default;
    if (
      !definition ||
      typeof definition !== "object" ||
      !("setup" in definition) ||
      typeof definition.setup !== "function"
    )
      throw new QimuError(
        "contract_mismatch",
        "module default export must be a Qimu unit declaration",
      );
    return definition as Unit;
  }
}

// ESM evaluation belongs to the Realm and cannot be cancelled or unloaded. A
// retiring execution stops observing it, releases its lease, and refuses late
// exports. The evaluation remains in the Realm cache until a controlled reload.
function observeEvaluation<T>(
  evaluation: Promise<T>,
  signal: AbortSignal,
): Promise<T> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(signal.reason);
      return;
    }
    const abort = () => {
      signal.removeEventListener("abort", abort);
      reject(signal.reason);
    };
    signal.addEventListener("abort", abort, { once: true });
    evaluation.then(
      (value) => {
        signal.removeEventListener("abort", abort);
        resolve(value);
      },
      (error: unknown) => {
        signal.removeEventListener("abort", abort);
        reject(error);
      },
    );
  });
}
