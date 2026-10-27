import type { ScannedImage, Severity, Tone } from "@/domain/model";

export const SEVERITIES: { key: Severity; tone: Tone; color: string }[] = [
  { key: "critical", tone: "err", color: "var(--destructive)" },
  { key: "high", tone: "orange", color: "var(--orange)" },
  { key: "medium", tone: "warn", color: "var(--warning)" },
  { key: "low", tone: "lime", color: "var(--lime)" },
  { key: "unknown", tone: "gray", color: "var(--muted-foreground)" },
];

export function totalOf(i: ScannedImage): number {
  return i.counts.critical + i.counts.high + i.counts.medium + i.counts.low + (i.counts.unknown ?? 0);
}

export function weightOf(i: ScannedImage): number {
  return i.counts.critical * 1000 + i.counts.high * 50 + i.counts.medium * 5 + i.counts.low;
}

export type RiskBand = "critical" | "high" | "medium" | "low" | "none";

/** Arcane-style patch priority: severity weighted, unused images count less. */
export function riskScore(images: ScannedImage[]): { score: number; band: RiskBand; tone: Tone; color: string } {
  let score = 0;
  for (const i of images) {
    const f = i.usage === "running" ? 1 : 0.4;
    score += f * (i.counts.critical * 12 + i.counts.high * 4 + i.counts.medium + i.counts.low * 0.2);
  }
  const s = Math.min(100, Math.round(score * 0.45));
  if (s >= 75) return { score: s, band: "critical", tone: "err", color: "var(--destructive)" };
  if (s >= 50) return { score: s, band: "high", tone: "orange", color: "var(--orange)" };
  if (s >= 25) return { score: s, band: "medium", tone: "warn", color: "var(--warning)" };
  if (s > 0) return { score: s, band: "low", tone: "lime", color: "var(--lime)" };
  return { score: 0, band: "none", tone: "ok", color: "var(--success)" };
}
