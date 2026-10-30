import { afterEach, expect, it, vi } from "vitest";
import { playCompletionChime } from "./chime";

class FakeAudioContext {
  static instances: FakeAudioContext[] = [];
  currentTime = 1;
  destination = {};
  resume = vi.fn(async () => {});
  close = vi.fn(async () => {});
  constructor() {
    FakeAudioContext.instances.push(this);
  }
  createOscillator() {
    return {
      type: "",
      frequency: { value: 0 },
      connect: (target: unknown) => target,
      start: vi.fn(),
      stop: vi.fn(),
    };
  }
  createGain() {
    return {
      gain: {
        setValueAtTime: vi.fn(),
        linearRampToValueAtTime: vi.fn(),
        exponentialRampToValueAtTime: vi.fn(),
      },
      connect: vi.fn(),
    };
  }
}

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  FakeAudioContext.instances = [];
});

it("closes its audio resource after the last note", async () => {
  vi.useFakeTimers();
  vi.stubGlobal("AudioContext", FakeAudioContext);
  const playing = playCompletionChime(new AbortController().signal);
  const audio = FakeAudioContext.instances[0]!;
  expect(audio.close).not.toHaveBeenCalled();
  await vi.advanceTimersByTimeAsync(411);
  await playing;
  expect(audio.close).toHaveBeenCalledOnce();
});

it("closes the audio context on withdrawal and joins the actual close", async () => {
  vi.useFakeTimers();
  vi.stubGlobal("AudioContext", FakeAudioContext);
  const controller = new AbortController();
  const closing = Promise.withResolvers<void>();
  const playing = playCompletionChime(controller.signal);
  const audio = FakeAudioContext.instances[0]!;
  audio.close.mockReturnValueOnce(closing.promise);
  const failure = new Error("notification source retired");
  controller.abort(failure);
  const rejected = expect(playing).rejects.toBe(failure);
  try {
    await Promise.resolve();
    expect(audio.close).toHaveBeenCalledOnce();
  } finally {
    closing.resolve();
    await rejected;
  }
  expect(vi.getTimerCount()).toBe(0);
});

it("refuses new audio after withdrawal and treats an unsupported channel as unavailable", async () => {
  vi.stubGlobal("AudioContext", FakeAudioContext);
  const controller = new AbortController();
  controller.abort();
  await expect(playCompletionChime(controller.signal)).rejects.toBe(
    controller.signal.reason,
  );
  expect(FakeAudioContext.instances).toEqual([]);
  vi.stubGlobal("AudioContext", undefined);
  await expect(
    playCompletionChime(new AbortController().signal),
  ).resolves.toBeUndefined();
});

it("retains both the playback and audio release causes", async () => {
  const audio = new FakeAudioContext();
  const playback = new Error("audio resume failed");
  const release = new Error("audio release failed");
  audio.resume.mockRejectedValueOnce(playback);
  audio.close.mockRejectedValueOnce(release);
  vi.stubGlobal(
    "AudioContext",
    vi.fn(function () {
      return audio;
    }),
  );
  await expect(
    playCompletionChime(new AbortController().signal),
  ).rejects.toMatchObject({ errors: [playback, release] });
});
