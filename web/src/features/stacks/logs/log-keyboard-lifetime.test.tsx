// Source: docklet-next/issues/177
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original keyboard row lifetime proofs for streaming logs.

import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { useRef, useState } from "react";
import type { LogLine } from "@/domain/model";
import { useLogKeyboard } from "./use-log-keyboard";
const lines = ["one", "two", "three"].map((id) => ({ id } as LogLine));
function Viewer({ rows }: { rows: LogLine[] }) {
  const [follow, setFollow] = useState(true); const scroller = useRef<HTMLDivElement>(null); const keys = useLogKeyboard(rows, follow, setFollow, scroller);
  return <div ref={scroller} {...keys.region} data-follow={follow}>{rows.map((line) => <div key={line.id} {...keys.row(line.id)}>{line.id}</div>)}</div>;
}
it.each(["filter", "stream eviction"])("WH-B4 %s clears focused ownership; Escape clears the row and End resumes the tail", async () => {
  const user = userEvent.setup(); const view = render(<Viewer rows={lines} />); const list = screen.getByRole("listbox");
  Object.defineProperty(list, "scrollHeight", { configurable: true, value: 800 });
  list.focus(); await user.keyboard("{ArrowDown}");
  expect(list.hasAttribute("aria-activedescendant")).toBe(true); expect(list.getAttribute("data-follow")).toBe("false");
  view.rerender(<Viewer rows={lines.slice(1)} />);
  expect(list.hasAttribute("aria-activedescendant")).toBe(false);
  view.rerender(<Viewer rows={lines} />); expect(list.hasAttribute("aria-activedescendant")).toBe(false);
  await user.keyboard("{ArrowUp}{Escape}"); expect(list.hasAttribute("aria-activedescendant")).toBe(false);
  expect(document.activeElement === list).toBe(true); expect(list.getAttribute("data-follow")).toBe("false");
  await user.keyboard("{ArrowDown}{End}"); expect(list.scrollTop).toBe(800); expect(list.getAttribute("data-follow")).toBe("true");
});
