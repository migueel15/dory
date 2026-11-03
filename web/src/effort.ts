// Remembers the last reasoning effort used with each model, per browser.
const key = (model: string) => `effort:${model}`;

export function savedEffort(model: string, values: string[]): string {
  try {
    const v = localStorage.getItem(key(model)) ?? "";
    return values.includes(v) ? v : "";
  } catch {
    return "";
  }
}

export function saveEffort(model: string, value: string) {
  try {
    localStorage.setItem(key(model), value);
  } catch {
    // Storage unavailable: the model default applies next time.
  }
}
