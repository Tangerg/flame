const noteGap = 0.11;
const noteDuration = 0.3;

export async function playCompletionChime(signal: AbortSignal): Promise<void> {
  signal.throwIfAborted();
  if (typeof AudioContext === "undefined") return;
  const audio = new AudioContext();
  const failures: unknown[] = [];
  let cleanup = () => {};
  try {
    await new Promise<void>((resolve, reject) => {
      // A suspended audio clock must not keep the installed source alive beyond its notes.
      const finish = setTimeout(resolve, (noteGap + noteDuration) * 1_000);
      const abort = () => reject(signal.reason);
      signal.addEventListener("abort", abort, { once: true });
      cleanup = () => {
        clearTimeout(finish);
        signal.removeEventListener("abort", abort);
      };
      void audio.resume().catch(reject);
      const now = audio.currentTime;
      [659.25, 987.77].forEach((frequency, index) => {
        const oscillator = audio.createOscillator();
        const gain = audio.createGain();
        oscillator.type = "sine";
        oscillator.frequency.value = frequency;
        const start = now + index * noteGap;
        gain.gain.setValueAtTime(0, start);
        gain.gain.linearRampToValueAtTime(0.11, start + 0.02);
        gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.26);
        oscillator.connect(gain).connect(audio.destination);
        oscillator.start(start);
        oscillator.stop(start + noteDuration);
      });
    });
  } catch (error) {
    failures.push(error);
  } finally {
    cleanup();
    try {
      await audio.close();
    } catch (error) {
      failures.push(error);
    }
  }
  if (failures.length === 1) throw failures[0];
  if (failures.length > 1)
    throw new AggregateError(
      failures,
      "Completion sound playback and release failed.",
    );
}
