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
