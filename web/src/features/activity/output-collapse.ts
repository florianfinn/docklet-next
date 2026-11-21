import type { RunOutputLine } from "@/domain/model";

/** One displayed output line; repeated progress frames of a layer fold into it. */
export interface OutputRow {
  line: RunOutputLine;
  /** Time of the first frame; the row keeps that position like a terminal line. */
  firstAt: string;
  /** Frames folded into this row, 1 for ordinary lines. */
  frames: number;
}

/**
 * Compose progress is stored as `{"log":"…"}` frames. Display, folding, copy and
 * download show the line itself, decoded once; any other frame stays verbatim.
 */
export function presentOutputText(text: string): string {
  if (!text.startsWith("{")) return text;
  try {
    const frame: unknown = JSON.parse(text);
    if (frame !== null && typeof frame === "object" && !Array.isArray(frame)) {
      const keys = Object.keys(frame);
      const log = (frame as { log?: unknown }).log;
      if (keys.length === 1 && keys[0] === "log" && typeof log === "string") return log.replace(/\r?\n$/, "");
    }
  } catch {
    // Not a frame; shown as it is.
  }
  return text;
}

const LAYER_STATUS = /^\s*([0-9a-f]{12})\s+(?:Pulling fs layer|Waiting|Downloading|Verifying Checksum|Download complete|Extracting|Pull complete|Already exists)\b/;

/**
 * Docker rewrites one terminal line per layer while it pulls. Lines of the
 * same layer within one command fold into a single row showing the newest
 * frame; every other line stays as it is, in chronological order.
 */
export function collapseOutput(lines: readonly RunOutputLine[]): OutputRow[] {
  const sorted = lines.map((line) => ({ ...line, text: presentOutputText(line.text) })).sort((a, b) => a.at.localeCompare(b.at));
  const rows: OutputRow[] = [];
  const open = new Map<string, number>();
  let command = 0;
  for (const line of sorted) {
    if (line.stream === "cmd") {
      command++;
      open.clear();
    }
    const layer = line.stream === "stderr" || line.stream === "stdout" ? LAYER_STATUS.exec(line.text)?.[1] : undefined;
    const key = layer && `${command}:${line.step ?? ""}:${layer}`;
    const index = key ? open.get(key) : undefined;
    if (key && index !== undefined) {
      const row = rows[index]!;
      rows[index] = { line, firstAt: row.firstAt, frames: row.frames + 1 };
      continue;
    }
    if (key) open.set(key, rows.length);
    rows.push({ line, firstAt: line.at, frames: 1 });
  }
  return rows;
}
