// Types for the vendored renderer (see PROVENANCE.md); only what the app uses.
export type LogoVariant = "mark" | "wordmark" | "full" | "compact" | "stacked" | "initial" | "dropcap";
export type LogoTone = "duo" | "brand" | "accent" | "mono";
export type LogoMotionStyle = "assembly" | "blueprint" | "layers" | "dock" | "relay";

export interface RenderLogoOptions {
  candidate?: string;
  variant?: LogoVariant;
  optical?: "regular" | "small";
  tone?: LogoTone;
  label?: string;
  decorative?: boolean;
  motionStyle?: LogoMotionStyle;
  animatable?: boolean;
  idPrefix?: string;
}

export const VARIANTS: readonly LogoVariant[];
export const MOTION_STYLES: readonly LogoMotionStyle[];
export const FAVORITE_LOGO: Readonly<{ candidate: string; variant: LogoVariant; tone: LogoTone; motionStyle: LogoMotionStyle }>;
export function renderLogoSVG(options?: RenderLogoOptions): string;
