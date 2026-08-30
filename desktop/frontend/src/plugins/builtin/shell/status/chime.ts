// Synthesized rather than an audio asset so there is nothing to bundle or decode. The
// CALLER owns the gate (toggle + focus), like osNotify. Browsers start an AudioContext
// suspended until a user gesture, but by run-completion the user has already clicked send;
// if it is somehow still suspended the notes silently don't sound.

let ctx: AudioContext | null = null;

function audioContext(): AudioContext | null {
  if (typeof AudioContext === "undefined") return null;
  ctx ??= new AudioContext();
  return ctx;
}

export function playCompletionChime(): void {
  const ac = audioContext();
  if (!ac) return;
  void ac.resume();

  const now = ac.currentTime;
  // A rising fifth, each note a sine with fast attack and exponential decay so it reads as
  // a chime rather than a beep.
  [659.25, 987.77].forEach((freq, i) => {
    const osc = ac.createOscillator();
    const gain = ac.createGain();
    osc.type = "sine";
    osc.frequency.value = freq;
    const start = now + i * 0.11;
    gain.gain.setValueAtTime(0, start);
    gain.gain.linearRampToValueAtTime(0.11, start + 0.02);
    gain.gain.exponentialRampToValueAtTime(0.0001, start + 0.26);
    osc.connect(gain).connect(ac.destination);
    osc.start(start);
    osc.stop(start + 0.3);
  });
}
