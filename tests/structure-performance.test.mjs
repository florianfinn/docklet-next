import test from "node:test";
import { renameSync, mkdirSync } from "node:fs";
import { resolve } from "node:path";
import { scratch, write, git, rawCommit, invoke, ok } from "./helpers.mjs";

test("3000-file staged rename and addition checks stay bounded", (t) => {
  const root = scratch(t);
  for (let i = 0; i < 3000; i++) write(root, `old/${i}.txt`, `base ${i}\n`);
  git(root, "add", ".");
  const base = rawCommit(root);
  mkdirSync(resolve(root, "new"));
  for (const scenario of ["rename", "addition"]) {
    git(root, "reset", "--hard", base);
    for (let i = 0; i < 3000; i++) {
      if (scenario === "rename") renameSync(resolve(root, `old/${i}.txt`), resolve(root, `new/${i}.txt`));
      else write(root, `new/${i}.txt`, `distinct ${i} public text\n`);
    }
    git(root, "add", ".");
    const start = performance.now();
    ok(invoke(root, process.execPath, ["scripts/check-structure.mjs", "--only", "size", "--staged", "--base", base], { timeout: 10000 }));
    t.diagnostic(`${scenario}: 3000 files, ${(performance.now() - start).toFixed(1)} ms (10 s process bound)`);
  }
});
