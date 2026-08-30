export type AudienceSource = "paste" | "file" | "leadads";
export type CampaignStatus = "draft" | "scheduled" | "done";

/** File/Lead Ads and campaign statuses are stored metadata — no vendor import/send. */
export function audienceSourceExecutes(source: AudienceSource): boolean {
  void source;
  return false;
}

export function campaignStatusExecutes(status: CampaignStatus): boolean {
  void status;
  return false;
}

export function audienceSourceNote(source: AudienceSource): "paste" | "file" | "leadads" {
  return source === "file" || source === "leadads" ? source : "paste";
}
