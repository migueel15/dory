import { storage, storageKeys } from "./lib/storage";
// Remembers the last model used in each launch section, per browser.
const key = (section: string) => storageKeys.lastModelOf(section);

export function savedModel(section: string): string {
  try {
    return storage.get(key(section)) ?? "";
  } catch {
    return "";
  }
}

export function saveModel(section: string, value: string) {
  try {
    storage.set(key(section), value);
  } catch {
    // Storage unavailable: the first model applies next time.
  }
}
