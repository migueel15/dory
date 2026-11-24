import { resource } from "./http";
import type { Memory } from "./types.gen";

export type MemoryInput = Partial<Memory> & { content: string };

export const memoriesApi = {
  memories: resource<Memory, MemoryInput>("/memories"),
};
