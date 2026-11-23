/** Sparkline coordinates in a 100 × 100 box (y grows downwards). */
export interface SparkPoint {
  x: number;
  y: number;
}

export function sparkPoints(data: number[], domain?: [number, number]): SparkPoint[] {
  if (data.length === 0) return [];
  const [min, max] = domain ?? [Math.min(...data), Math.max(...data)];
  const span = max - min || 1;
  const step = data.length > 1 ? 100 / (data.length - 1) : 0;
  return data.map((value, i) => {
    const clamped = Math.min(max, Math.max(min, value));
    return { x: data.length > 1 ? i * step : 50, y: 100 - ((clamped - min) / span) * 100 };
  });
}

export function linePath(points: SparkPoint[]): string {
  return points.map((p, i) => `${i ? "L" : "M"}${p.x.toFixed(2)} ${p.y.toFixed(2)}`).join(" ");
}

export function areaPath(points: SparkPoint[]): string {
  const first = points[0];
  const last = points.at(-1);
  if (!first || !last) return "";
  return `${linePath(points)} L${last.x.toFixed(2)} 100 L${first.x.toFixed(2)} 100 Z`;
}

export interface SparkBar { value: number; placeholder: boolean }

/** Newest samples on the right; with `slots`, the left gap is filled with the mean of the real samples. */
export function barSlots(data: readonly number[], slots?: number): SparkBar[] {
  const real = slots ? data.slice(-slots) : data;
  if (!real.length) return [];
  const mean = real.reduce((sum, v) => sum + v, 0) / real.length;
  const gap = slots ? slots - real.length : 0;
  return [...Array.from({ length: gap }, () => ({ value: mean, placeholder: true })), ...real.map((value) => ({ value, placeholder: false }))];
}
