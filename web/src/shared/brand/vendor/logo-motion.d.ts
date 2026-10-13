// Types for the vendored motion plans (see PROVENANCE.md); only what the app uses.
import type { LogoMotionStyle, LogoVariant } from "./logo-svg";

export interface MotionStep {
  group: string;
  selector: string;
  keyframes: Keyframe[];
  delay: number;
  duration: number;
  origin?: string;
  label: string | null;
}

export const DURATION_RANGES: Readonly<Record<LogoMotionStyle, Readonly<{ min: number; max: number }>>>;
export function resolveStyle(candidateId: string, style: string): LogoMotionStyle;
export function defaultDuration(candidateId: string, style?: LogoMotionStyle): number;
export function clampDuration(ms: number, fallback: number, style?: LogoMotionStyle): number;
export function motionPlan(candidateId: string, variant: LogoVariant, totalMs: number, options?: { style?: LogoMotionStyle; optical?: "regular" | "small" }): MotionStep[];
