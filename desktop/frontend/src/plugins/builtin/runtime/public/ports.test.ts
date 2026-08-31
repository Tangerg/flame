import { describe, expect, it } from "vitest";
import {
  followRuntimeGeneration,
  RuntimeConnectionGeneration,
  type RuntimeStreamPorts,
} from "./ports";

// The comparison is the whole point: `subscribeConnection` fires on connection activity, not
// only on replacement, so a follower acting on every notification retires in-flight mutations
// against a generation that never moved — work the person just started, dropped for nothing.

function fakePorts(): RuntimeStreamPorts & { emit(): void; set(next: string | null): void } {
  let generation: RuntimeConnectionGeneration | null = null;
  const listeners = new Set<() => void>();
  return {
    connectionGeneration: () => generation,
    subscribeConnection: (onChange) => {
      listeners.add(onChange);
      return () => listeners.delete(onChange);
    },
    reportConnectionLoss: () => Promise.resolve(),
    emit: () => listeners.forEach((listener) => listener()),
    set: (next) => {
      generation = next === null ? null : RuntimeConnectionGeneration.forProcess(next);
    },
  };
}

describe("following the runtime connection generation", () => {
  it("stays silent while the generation has not moved", () => {
    const ports = fakePorts();
    ports.set("process-1");
    const seen: Array<RuntimeConnectionGeneration | null> = [];
    followRuntimeGeneration(ports, (generation) => seen.push(generation));

    ports.emit();
    ports.emit();
    ports.emit();

    expect(seen).toEqual([]);
  });

  it("reports a replacement exactly once", () => {
    const ports = fakePorts();
    ports.set("process-1");
    const seen: Array<RuntimeConnectionGeneration | null> = [];
    followRuntimeGeneration(ports, (generation) => seen.push(generation));

    ports.set("process-2");
    ports.emit();
    ports.emit();

    expect(seen).toHaveLength(1);
    expect(seen[0]?.belongsTo("process-2")).toBe(true);
  });

  it("reports losing the connection, which is not the same as keeping it", () => {
    const ports = fakePorts();
    ports.set("process-1");
    const seen: Array<RuntimeConnectionGeneration | null> = [];
    followRuntimeGeneration(ports, (generation) => seen.push(generation));

    ports.set(null);
    ports.emit();
    ports.emit();

    expect(seen).toEqual([null]);
  });

  // Identity is deliberately object identity, so a reconnection to the SAME process is still
  // a successor: its in-flight work belongs to a stream that no longer exists.
  it("reports a successor built for the same process id", () => {
    const ports = fakePorts();
    ports.set("process-1");
    const seen: Array<RuntimeConnectionGeneration | null> = [];
    followRuntimeGeneration(ports, (generation) => seen.push(generation));

    ports.set("process-1");
    ports.emit();

    expect(seen).toHaveLength(1);
    expect(seen[0]?.belongsTo("process-1")).toBe(true);
  });

  it("stops reporting once the follower is disposed", () => {
    const ports = fakePorts();
    ports.set("process-1");
    const seen: Array<RuntimeConnectionGeneration | null> = [];
    const stop = followRuntimeGeneration(ports, (generation) => seen.push(generation));

    stop();
    ports.set("process-2");
    ports.emit();

    expect(seen).toEqual([]);
  });
});
