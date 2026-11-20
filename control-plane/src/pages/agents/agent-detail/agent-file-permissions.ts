import type { AgentData } from "@/types/agent";

export function canManageAgentFiles(
  agent: Pick<AgentData, "agent_type" | "owner_id">,
  userId: string,
  isSystemOwner: boolean,
): boolean {
  if (agent.agent_type !== "predefined") return true;
  return isSystemOwner || agent.owner_id === userId;
}
