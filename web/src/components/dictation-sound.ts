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
    const gain = context.createGain();
    const now = context.currentTime;
    const end = now + 0.16;
    oscillator.frequency.setValueAtTime(action === "start" ? 520 : 740, now);
    oscillator.frequency.exponentialRampToValueAtTime(action === "start" ? 740 : 520, end);
    gain.gain.setValueAtTime(0, now);
    gain.gain.linearRampToValueAtTime(0.16, now + 0.015);
    gain.gain.exponentialRampToValueAtTime(0.001, end);
    oscillator.connect(gain).connect(context.destination);
    currentTone = oscillator;
    oscillator.onended = () => {
      oscillator.disconnect();
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
