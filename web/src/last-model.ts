// Remembers the last model used in each launch section, per browser.
const key = (section: string) => `dory.last-model:${section}`;

export function savedModel(section: string): string {
  try {
    return localStorage.getItem(key(section)) ?? "";
  } catch {
    return "";
  }
}

export function saveModel(section: string, value: string) {
  try {
    localStorage.setItem(key(section), value);
  } catch {
    // Storage unavailable: the first model applies next time.
  }
}
