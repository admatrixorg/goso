import { Badge } from "./Badge";

/** Marks every mock/wireframe surface. Never present live CRM numbers as real. */
export function DemoBadge() {
  return <Badge tone="warning">DEMO</Badge>;
}
