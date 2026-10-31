// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original development-only scripted echo terminal; executes no commands.

import type { OpenShell } from "../shell";
export const openScriptedShell: OpenShell = (_id, options, push) => {
  let active = true, line = "", cols = options.cols ?? 80, rows = options.rows ?? 24;
  const output = (data: string) => { if (active) push({ type: "output", data: new TextEncoder().encode(data) }); };
  queueMicrotask(() => { if (active) { push({ type: "live" }); output("$ "); } });
  return {
    send(data) {
      if (!active) return;
      for (const character of data) {
        if (character === "\r" || character === "\n") {
          output("\r\n");
          if (line === "exit") { push({ type: "exit", code: 0, reason: "" }); active = false; return; }
          if (line === "stty size") output(`${rows} ${cols}\r\n`);
          else if (line.startsWith("echo ")) output(`${line.slice(5)}\r\n`);
          line = ""; output("$ ");
        } else if (character === "\u007f") { line = line.slice(0, -1); output("\b \b"); }
        else { line += character; output(character); }
      }
    },
    resize(nextCols, nextRows) { cols = nextCols; rows = nextRows; },
    close() { active = false; },
  };
};
