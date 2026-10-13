/** Fixture clock: all synthetic timestamps are relative to module load. */
export const FIXTURE_NOW = Date.now();

export function minutesAgo(min: number): string {
  return new Date(FIXTURE_NOW - min * 60_000).toISOString();
}

export function hoursAgo(h: number): string {
  return minutesAgo(h * 60);
}

export function daysAgo(d: number): string {
  return minutesAgo(d * 1440);
}

export function minutesAhead(min: number): string {
  return new Date(FIXTURE_NOW + min * 60_000).toISOString();
}
