import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const record = readFileSync(new URL("../docs/design/edge-threat-review.md", import.meta.url), "utf8");
for (const threat of ["T6", "T8", "T11"]) {
  test(`${threat} distinguishes pre-registration baseline claims from current decisions`, () => {
    const section = record.split(new RegExp(`^### ${threat}\\.`, "m"))[1]?.split(/^### /m)[0];
    assert.ok(section, `${threat} section must exist`);
    assert.ok(section.includes("(baseline before #81)"), `${threat} baseline must be explicitly dated`);
  });
}
