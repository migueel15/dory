import { Environments } from "../pages/crud";
import type { SectionProps } from "./settings-node";

// ConnectionsSettings is the "Conexiones" section. For now it hosts the
// environments view; a later phase reworks it. It ignores the node selector.
export function ConnectionsSettings(_: SectionProps) {
  return <Environments />;
}
