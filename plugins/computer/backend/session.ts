import { Refusal } from "@qimu/node";
import type { ControlSession, ExecuteInput } from "../contracts/capability";

type Reason = NonNullable<ControlSession["reason"]>;
type Operation = NonNullable<ControlSession["operation"]>;
type Phase =
  | { state: "opening" }
  | { state: "ready"; surfaceDigest: string }
  | { state: "executing"; surfaceDigest: string; operation: Operation }
  | { state: "stopping"; reason: Reason; operation?: Operation }
  | { state: "closed" | "faulted"; reason: Reason };

export class Session {
  readonly id: string;
  readonly #openedAt: string;
  readonly #expiresAt: string;
  #workerId: string;
  #phase: Phase = { state: "opening" };

  constructor(
    id: string,
    workerId: string,
    openedAt: string,
    expiresAt: string,
  ) {
    if (
      !id ||
      !workerId ||
      !Number.isFinite(Date.parse(openedAt)) ||
      Date.parse(openedAt) >= Date.parse(expiresAt) ||
      !Number.isFinite(Date.parse(expiresAt))
    )
      throw new Error("computer: invalid control session");
    this.id = id;
    this.#workerId = workerId;
    this.#openedAt = openedAt;
    this.#expiresAt = expiresAt;
  }

  snapshot(): ControlSession {
    return structuredClone({
      id: this.id,
      workerId: this.#workerId,
      openedAt: this.#openedAt,
      expiresAt: this.#expiresAt,
      ...this.#phase,
    });
  }

  connected(surfaceDigest: string): void {
    if (
      this.#phase.state !== "opening" ||
      !/^[a-f0-9]{64}$/.test(surfaceDigest)
    )
      throw new Error("computer: connection cannot advance this session");
    this.#phase = { state: "ready", surfaceDigest };
  }

  assertCurrent(id: string): void {
    if (id !== this.id)
      throw new Refusal(
        "session_replaced",
        "computer control session is no longer current",
      );
  }

  expired(now: number): boolean {
    return now >= Date.parse(this.#expiresAt);
  }

  canRevoke(id: string, operationId?: string): boolean {
    return (
      id === this.id &&
      ["opening", "ready", "executing"].includes(this.#phase.state) &&
      (operationId === undefined ||
        (this.#phase.state === "executing" &&
          this.#phase.operation.id === operationId))
    );
  }

  begin(input: ExecuteInput, operation: Operation): void {
    this.assertCurrent(input.sessionId);
    if (this.#phase.state !== "ready")
      throw new Refusal(
        "computer_unavailable",
        "computer control session is not ready for another operation",
      );
    if (
      input.workerId !== this.#workerId ||
      input.surfaceDigest !== this.#phase.surfaceDigest
    )
      throw new Refusal(
        "worker_replaced",
        "read the current computer context before using this worker",
      );
    this.#phase = {
      state: "executing",
      surfaceDigest: this.#phase.surfaceDigest,
      operation,
    };
  }

  complete(id: string, workerId?: string): void {
    if (
      this.#phase.state === "stopping" ||
      this.#phase.state === "faulted" ||
      this.#phase.state === "closed"
    )
      return;
    if (this.#phase.state !== "executing" || this.#phase.operation.id !== id)
      throw new Error("computer: operation does not own this completion");
    if (workerId) this.#workerId = workerId;
    this.#phase = { state: "ready", surfaceDigest: this.#phase.surfaceDigest };
  }

  stopping(reason: Reason, operationId?: string): void {
    if (operationId) this.assertOperation(operationId);
    if (this.#phase.state === "closed" || this.#phase.state === "stopping")
      return;
    this.#phase = {
      state: "stopping",
      reason,
      ...(this.#phase.state === "executing"
        ? { operation: this.#phase.operation }
        : {}),
    };
  }

  assertOperation(id: string): void {
    if (this.#phase.state !== "executing" || this.#phase.operation.id !== id)
      throw new Refusal(
        "operation_retired",
        "computer operation is no longer current",
      );
  }

  retired(reason: Reason, clean: boolean): void {
    this.#phase = { state: clean ? "closed" : "faulted", reason };
  }
}
