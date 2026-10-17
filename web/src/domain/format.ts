import { activeLocale, formatNumber, INTL_LOCALE, type AppLocale } from "./locale";

export { formatNumber };

/** Words the formatters add themselves; Intl covers everything else. */
const WORDS: Record<AppLocale, { justNow: string; soon: string; now: string; day: string; days: string; percentGap: string }> = {
  de: { justNow: "gerade eben", soon: "gleich", now: "jetzt", day: "Tag", days: "Tagen", percentGap: " " },
  "en-US": { justNow: "just now", soon: "shortly", now: "now", day: "day", days: "days", percentGap: "" },
};

const formatters = new Map<string, Intl.DateTimeFormat | Intl.RelativeTimeFormat>();

function dateFormat(options: Intl.DateTimeFormatOptions): Intl.DateTimeFormat {
  const locale = INTL_LOCALE[activeLocale()];
  const key = `${locale}|d|${JSON.stringify(options)}`;
  let f = formatters.get(key) as Intl.DateTimeFormat | undefined;
  if (!f) formatters.set(key, (f = new Intl.DateTimeFormat(locale, options)));
  return f;
}

function relativeFormat(): Intl.RelativeTimeFormat {
  const locale = INTL_LOCALE[activeLocale()];
  let f = formatters.get(`${locale}|r`) as Intl.RelativeTimeFormat | undefined;
  if (!f) formatters.set(`${locale}|r`, (f = new Intl.RelativeTimeFormat(locale, { numeric: "auto" })));
  return f;
}

export function formatTime(iso: string, seconds = false): string {
  return dateFormat(seconds ? { hour: "2-digit", minute: "2-digit", second: "2-digit" } : { hour: "2-digit", minute: "2-digit" }).format(new Date(iso));
}

export function formatDate(iso: string): string {
  return dateFormat({ day: "2-digit", month: "2-digit" }).format(new Date(iso));
}

export function formatDateTime(iso: string): string {
  return dateFormat({ day: "2-digit", month: "2-digit", hour: "2-digit", minute: "2-digit" }).format(new Date(iso));
}

export function formatRelative(iso: string, now = Date.now()): string {
  const w = WORDS[activeLocale()];
  const rtf = relativeFormat();
  const diff = new Date(iso).getTime() - now;
  if (!Number.isFinite(diff)) return "–";
  const abs = Math.abs(diff);
  if (abs < 45_000) return diff <= 0 ? w.justNow : w.soon;
  if (abs < 3_600_000) return rtf.format(Math.round(diff / 60_000), "minute");
  if (abs < 86_400_000) return rtf.format(Math.round(diff / 3_600_000), "hour");
  return rtf.format(Math.round(diff / 86_400_000), "day");
}

/** "in 3 h 12 min" style countdown for schedules. */
export function formatCountdown(iso: string, now = Date.now()): string {
  const w = WORDS[activeLocale()];
  const ms = Math.max(0, new Date(iso).getTime() - now);
  const min = Math.round(ms / 60_000);
  if (min < 1) return w.now;
  if (min < 60) return `in ${min} min`;
  const h = Math.floor(min / 60);
  const m = min % 60;
  if (h < 24) return m ? `in ${h} h ${m} min` : `in ${h} h`;
  const d = Math.floor(h / 24);
  return `in ${d} ${d === 1 ? w.day : w.days}`;
}

export function formatMem(mb: number): string {
  if (mb >= 1024) return `${formatNumber(mb / 1024, { maximumFractionDigits: 1 })} GB`;
  return `${Math.round(mb)} MB`;
}

export function formatGb(gb: number): string {
  if (gb >= 1000) return `${formatNumber(gb / 1000, { maximumFractionDigits: 1 })} TB`;
  return `${Math.round(gb)} GB`;
}

export function formatPercent(value: number): string {
  return `${Math.round(value)}${WORDS[activeLocale()].percentGap}%`;
}

export function formatPorts(ports: { host?: string; container: string; protocol?: string }[]): string {
  return ports.map((p) => (p.host ? `${p.host}→${p.container}` : p.container) + (p.protocol === "udp" ? "/udp" : "")).join(", ");
}

export function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${formatNumber(v, { maximumFractionDigits: v < 10 ? 1 : 0 })} ${units[i]}`;
}
