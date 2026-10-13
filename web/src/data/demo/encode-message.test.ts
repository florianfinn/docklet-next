import { describe, expect, it } from "vitest";

import { catalogs } from "@/app/i18n/catalog";
import { renderMessage } from "@/data/message-code";

import baseline from "./fixtures/legacy-run-messages.json";
import { encodeDemoMessage } from "./encode-message";
import { EXACT, PATTERNS } from "./demo-text/en-us";
import { EXACT_CODES, PATTERN_CODES } from "./message-codes";

describe("coded run and event messages", () => {
  it("keeps the code inventory aligned with the demo producers", () => {
    expect(Object.keys(EXACT_CODES).sort()).toEqual(Object.keys(EXACT).sort());
    expect(PATTERN_CODES.length).toBe(79);
    expect(PATTERNS.length).toBeGreaterThanOrEqual(PATTERN_CODES.length);
  });
  it("renders every exact code in both languages with the captured text", () => {
    const seen = new Set<string>();
    for (const { de, en } of baseline.exact) {
      const message = encodeDemoMessage(de);
      seen.add(message.code);
      expect(renderMessage(message, "de"), message.code).toBe(de);
      expect(renderMessage(message, "en-US"), message.code).toBe(en);
    }
    expect(seen.size).toBe(Object.keys(catalogs.de.runMessages.exact).length);
  });

  it("renders every parameterized code and optional branch with the captured text", () => {
    const seen = new Set<string>();
    for (const { de, en } of baseline.samples) {
      const message = encodeDemoMessage(de);
      if (message.code.startsWith("runMessages.pattern.")) seen.add(message.code);
      expect(renderMessage(message, "de"), message.code).toBe(de);
      expect(renderMessage(message, "en-US"), message.code).toBe(en);
    }
    expect(seen.size).toBe(Object.keys(catalogs.de.runMessages.pattern).length);
  });
});
