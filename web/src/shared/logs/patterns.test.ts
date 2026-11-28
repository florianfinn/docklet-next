// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import { expect, it } from "vitest";
import type { LogLine } from "@/domain/model";
import { matchesPattern, PatternFold, patternKey, PATTERN_LINES, PATTERN_BYTES } from "./patterns";
import { PatternQueue } from "./pattern-queue";
import { logFixture, settleLogs } from "@/data/http/logs.test-support";

export const line = (message: string, service = "A", i = 0): LogLine => ({ id: String(i), ts: new Date(1767225600000 + i * 1000).toISOString(), service, message, level: "info" });
const word = (i: number): string => i ? word(Math.floor(i / 26)) + String.fromCharCode(97 + i % 26) : "a";
it("joins ANSI stack traces per interleaved source and shares the Raw key pipeline", () => {
  const fold = new PatternFold();
  const a = line('\x1b[31mERROR request 42 from 192.0.2.1 id abcdef1234 "alice"\x1b[0m');
  [a, line("WARN disconnected", "B"), line("  at worker", "A"), line("Caused by retry", "B"), line('File "example.py"', "A"), line('ERROR request 99 from 192.0.2.2 id abcdef9999 "bob"', "A", 3)].forEach((value) => fold.add(value));
  const rows = fold.snapshot();
  expect(rows.map((row) => row.count)).toEqual([4, 2]);
  expect(rows[0]!.level).toBe("error"); expect(rows[1]!.level).toBe("warn");
  expect(rows[0]!.samples).toHaveLength(2);
  expect(rows[0]!.bins).toHaveLength(30); expect(rows[0]!.bins.reduce((a, b) => a + b)).toBe(4);
  expect([a, line(" at worker"), line("WARN disconnected")].filter((value) => patternKey(value) === rows[0]!.key)).toEqual([a]);
  expect(patternKey(line("2026-01-01T12:34:56Z id 550e8400-e29b-41d4-a716-446655440000 from 2001:db8::1"))).toBe("<time> id <uuid> from <ip>");
});
it("caps 6000 distinct word shapes with conserved overflow counts", () => {
  const fold = new PatternFold();
  for (let i = 0; i < 6000; i++) fold.add(line(`shape ${word(i)}`));
  const rows = fold.snapshot();
  expect(rows.filter((row) => !row.other)).toHaveLength(5000);
  expect(rows.length).toBe(5001); expect(rows.find((row) => row.other)?.count).toBe(1000);
  expect(rows.reduce((n, row) => n + row.count, 0)).toBe(6000);
});
it("bounds queued folding at 100000 even if a producer crosses the cap", () => {
  const queue = new PatternQueue();
  queue.enqueue(Array.from({ length: PATTERN_LINES + 1 }, () => line("é")));
  while (queue.pending) queue.slice(() => 0);
  expect(queue.fold.lines).toBe(100000); expect(queue.fold.bytes).toBe(200000);
  expect(queue.fold.snapshot()[0]?.count).toBe(100000);
});
it("folds exactly bounded real transport input at the UTF-8 mid-frame byte cap", async () => {
  const fixture = logFixture(), queue = new PatternQueue();
  const read = fixture.owner.readLogs({ kind: "container", id: "0~example" }, { maxLines: PATTERN_LINES, maxBytes: 5 }, (lines) => queue.enqueue(lines), new AbortController().signal);
  await settleLogs(); fixture.sockets[0]!.send([1, 2, 3].map((seq) => ({ seq, message: "é", timestamp: "2026-01-01T00:00:00Z", level: "info" })));
  expect(await read).toEqual({ lines: 2, bytes: 4, reason: "cap-bytes" });
  queue.slice(() => 0); expect(queue.fold.lines).toBe(2); expect(queue.fold.bytes).toBe(4);
  const bounded = new PatternQueue(); bounded.enqueue(Array.from({ length: 22 }, () => line("é".repeat(512 * 1024))));
  while (bounded.pending) bounded.slice(() => 0);
  expect(bounded.fold.bytes).toBe(PATTERN_BYTES); expect(bounded.fold.lines).toBe(20);
});
it("drops queued frames on clear and stops a slice before eight milliseconds", () => {
  const queue = new PatternQueue(); queue.enqueue([line("one"), line("two"), line("three")]);
  let clock = 0; queue.slice(() => clock++ * 3);
  expect(queue.fold.lines).toBe(2); expect(queue.pending).toBe(1);
  queue.clear(); expect(queue.pending).toBe(0); queue.enqueue([line("late")]); queue.slice(() => 0);
  expect(queue.fold.lines).toBe(2);
});

it("keeps JSON and logfmt levels apart and keeps JSON keys and messages in the shape", () => {
  const fold = new PatternFold();
  const info = line('{"level":"info","msg":"request served","path":"/a","ms":12}', "A", 0);
  const errors = [line('{"level":"error","msg":"request served","path":"/b","ms":40}', "A", 1), line('{"level":"error","msg":"request served","path":"/c","ms":41}', "A", 2)];
  const other = line('{"level":"info","msg":"cache warmed","path":"/a","ms":3}', "A", 3);
  const logfmt = [line('level="error" msg="disk full"', "A", 4), line('level="info" msg="disk full"', "A", 5)];
  [info, ...errors, other, ...logfmt].forEach((value) => fold.add(value));
  const rows = fold.snapshot();
  const json = rows.filter((row) => row.key.startsWith("{"));
  expect(json.map((row) => `${row.level}:${row.count}`).sort()).toEqual(["error:2", "info:1", "info:1"]);
  expect(json.some((row) => row.key.includes('"msg":"cache warmed"'))).toBe(true);
  const quotedLevels = rows.filter((row) => row.key.startsWith("level="));
  expect(quotedLevels.map((row) => row.level).sort()).toEqual(["error", "info"]);
  const errorRow = json.find((row) => row.level === "error")!;
  expect(matchesPattern(errors[0]!, errorRow)).toBe(true);
  expect(matchesPattern(info, errorRow)).toBe(false);
});
