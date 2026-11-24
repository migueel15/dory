let output: AudioContext | undefined;
let currentTone: OscillatorNode | undefined;

export function prepareDictationSound() {
  try {
    output ??= new AudioContext();
    if (output.state !== "running") void output.resume().catch(() => {});
  } catch {
    // Audio feedback is optional.
  }
}

export async function playDictationSound(action: "start" | "stop") {
  const context = output;
  if (!context) return;
  try {
    if (context.state !== "running") await context.resume();

    const oscillator = context.createOscillator();
    const filter = context.createBiquadFilter();
    const gain = context.createGain();
    const now = context.currentTime;
    const duration = 0.2;
    const end = now + duration;
    const [from, to] = action === "start" ? [300, 480] : [480, 300];
    filter.type = "lowpass";
    filter.frequency.value = 900;
    oscillator.frequency.setValueAtTime(from, now);
    oscillator.frequency.exponentialRampToValueAtTime(to, now + duration * 0.7);
    gain.gain.setValueAtTime(0, now);
    gain.gain.linearRampToValueAtTime(0.13, now + 0.03);
    gain.gain.exponentialRampToValueAtTime(0.0004, end);
    oscillator.connect(filter).connect(gain).connect(context.destination);
    currentTone = oscillator;
    oscillator.onended = () => {
      oscillator.disconnect();
      filter.disconnect();
      gain.disconnect();
      if (currentTone !== oscillator) return;
      currentTone = undefined;
      void context.suspend().catch(() => {});
    };
    oscillator.start(now);
    oscillator.stop(end);
  } catch {
    // The microphone and transcription can continue without audio feedback.
  }
}
