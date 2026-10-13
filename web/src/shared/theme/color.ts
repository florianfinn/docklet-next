// sRGB ⇄ OKLCH conversion and WCAG contrast (formulas after Björn Ottosson's OKLab and WCAG 2.2).

export interface Lch {
  l: number;
  c: number;
  h: number;
}

interface Rgb {
  r: number;
  g: number;
  b: number;
}

const toLinear = (c: number) => (c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4);
const fromLinear = (c: number) => (c <= 0.0031308 ? 12.92 * c : 1.055 * c ** (1 / 2.4) - 0.055);

export function isHex(value: string): boolean {
  return /^#[0-9a-f]{6}$/i.test(value);
}

/** Accepts #rgb, #rrggbb and the same without "#"; returns lower-case #rrggbb. */
export function normalizeHex(value: string): string | null {
  const v = value.trim().replace(/^#/, "").toLowerCase();
  if (/^[0-9a-f]{3}$/.test(v)) return `#${[...v].map((c) => c + c).join("")}`;
  return /^[0-9a-f]{6}$/.test(v) ? `#${v}` : null;
}

function hexToRgb(hex: string): Rgb {
  const n = Number.parseInt(hex.slice(1), 16);
  return { r: ((n >> 16) & 255) / 255, g: ((n >> 8) & 255) / 255, b: (n & 255) / 255 };
}

function linearToOklab({ r, g, b }: Rgb) {
  const l = Math.cbrt(0.4122214708 * r + 0.5363325363 * g + 0.0514459929 * b);
  const m = Math.cbrt(0.2119034982 * r + 0.6806995451 * g + 0.1073969566 * b);
  const s = Math.cbrt(0.0883024619 * r + 0.2817188376 * g + 0.6299787005 * b);
  return { L: 0.2104542553 * l + 0.793617785 * m - 0.0040720468 * s, a: 1.9779984951 * l - 2.428592205 * m + 0.4505937099 * s, b: 0.0259040371 * l + 0.7827717662 * m - 0.808675766 * s };
}

function oklchToLinear({ l, c, h }: Lch): Rgb {
  const rad = (h * Math.PI) / 180;
  const a = c * Math.cos(rad);
  const b = c * Math.sin(rad);
  const l_ = (l + 0.3963377774 * a + 0.2158037573 * b) ** 3;
  const m_ = (l - 0.1055613458 * a - 0.0638541728 * b) ** 3;
  const s_ = (l - 0.0894841775 * a - 1.291485548 * b) ** 3;
  return { r: 4.0767416621 * l_ - 3.3077115913 * m_ + 0.2309699292 * s_, g: -1.2684380046 * l_ + 2.6097574011 * m_ - 0.3413193965 * s_, b: -0.0041960863 * l_ - 0.7034186147 * m_ + 1.707614701 * s_ };
}

export function hexToOklch(hex: string): Lch {
  const { r, g, b } = hexToRgb(hex);
  const lab = linearToOklab({ r: toLinear(r), g: toLinear(g), b: toLinear(b) });
  const c = Math.hypot(lab.a, lab.b);
  const h = c < 1e-4 ? 0 : ((Math.atan2(lab.b, lab.a) * 180) / Math.PI + 360) % 360;
  return { l: lab.L, c, h };
}

const inGamut = ({ r, g, b }: Rgb) => [r, g, b].every((v) => v >= -1e-4 && v <= 1 + 1e-4);

/** Linear sRGB of an OKLCH colour; chroma is reduced until it fits the sRGB gamut. */
function toDisplayLinear(color: Lch): Rgb {
  let lo = 0;
  let hi = color.c;
  let rgb = oklchToLinear(color);
  if (!inGamut(rgb)) {
    for (let i = 0; i < 24; i++) {
      const mid = (lo + hi) / 2;
      if (inGamut(oklchToLinear({ ...color, c: mid }))) lo = mid;
      else hi = mid;
    }
    rgb = oklchToLinear({ ...color, c: lo });
  }
  const clamp = (v: number) => Math.min(1, Math.max(0, v));
  return { r: clamp(rgb.r), g: clamp(rgb.g), b: clamp(rgb.b) };
}

export function oklchToHex(color: Lch): string {
  const { r, g, b } = toDisplayLinear(color);
  return `#${[r, g, b].map((v) => Math.round(fromLinear(v) * 255).toString(16).padStart(2, "0")).join("")}`;
}

export function oklchCss({ l, c, h }: Lch): string {
  return `oklch(${l.toFixed(3)} ${c.toFixed(4)} ${h.toFixed(2)})`;
}

function luminance(color: Lch): number {
  const { r, g, b } = toDisplayLinear(color);
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** WCAG contrast ratio between two colours (1 … 21). */
export function contrast(a: Lch, b: Lch): number {
  const [x, y] = [luminance(a), luminance(b)].sort((p, q) => q - p) as [number, number];
  return (x + 0.05) / (y + 0.05);
}
