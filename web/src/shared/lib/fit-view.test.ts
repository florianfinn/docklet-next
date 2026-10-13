import { describe, expect, it } from "vitest";

import { INITIAL_FIT, nextFit } from "./fit-view";
import { sortRows } from "./table-sort";

describe("overview fit", () => {
  it("steps from table to condensed to cards when content overflows and back with hysteresis", () => {
    // Table needs 1 400 px in a 1 100 px container.
    let s = nextFit(INITIAL_FIT, 1100, 1400);
    expect(s).toEqual({ view: "condensed", need: { table: 1400 } });
    // Condensed fits (content = container width): stays.
    s = nextFit(s, 1100, 1100);
    expect(s.view).toBe("condensed");
    // Narrower than the condensed table needs: cards.
    s = nextFit(s, 700, 820);
    expect(s).toEqual({ view: "cards", need: { table: 1400, condensed: 820 } });
    // Cards never overflow; a wider container returns to the widest view that fits its recorded need.
    expect(nextFit(s, 900, 900).view).toBe("condensed");
    expect(nextFit(s, 1500, 1500).view).toBe("table");
    // Without layout (tests, hidden) nothing changes.
    expect(nextFit(INITIAL_FIT, 0, 0)).toBe(INITIAL_FIT);
  });

  it("can skip the condensed view", () => {
    expect(nextFit(INITIAL_FIT, 500, 900, ["table", "cards"]).view).toBe("cards");
  });

  it("sorts unknown values last in both directions", () => {
    const rows = [
      { id: "a", v: null },
      { id: "b", v: 3 },
      { id: "c", v: 12 },
      { id: "d", v: null },
    ];
    const id = (r: { id: string }) => ({ id: r.id, name: r.id, hostName: "h" });
    expect(sortRows(rows, { key: "v", dir: "asc" }, (r) => r.v, id).map((r) => r.id)).toEqual(["b", "c", "a", "d"]);
    expect(sortRows(rows, { key: "v", dir: "desc" }, (r) => r.v, id).map((r) => r.id)).toEqual(["c", "b", "a", "d"]);
  });
});
