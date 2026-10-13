import { execFileSync } from "node:child_process";
import { readFileSync, lstatSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

export const ROOT = process.cwd();
export function git(args, root = ROOT, options = {}) {
  return execFileSync("git", args, {
    cwd: root,
    encoding: "utf8",
    stdio: ["pipe", "pipe", "pipe"],
    maxBuffer: 128 * 1024 * 1024,
    ...options,
  });
}
export const tracked = (root = ROOT) => git(["ls-files", "-z"], root).split("\0").filter(Boolean);
export const json = (path) => JSON.parse(readFileSync(path, "utf8"));
export function entries(root = ROOT, revision) {
  const args = revision ? ["ls-tree", "-rz", "--full-tree", revision] : ["ls-files", "--stage", "-z"];
  return git(args, root)
    .split("\0")
    .filter(Boolean)
    .map((row) => {
      const at = row.indexOf("\t");
      const path = row.slice(at + 1);
      const [mode, second, third] = row.slice(0, at).split(" ");
      if (!revision && third !== "0") throw new Error("index-conflict");
      if (revision && second !== "blob") throw new Error("unsupported-git-entry");
      return { path, mode, oid: revision ? third : second };
    });
}
export function files(root = ROOT, { staged = false, revision } = {}) {
  const list = entries(root, revision);
  if (staged || revision) {
    if (!list.length) return [];
    // One batch avoids a process per blob on large import commits.
    const output = git(["cat-file", "--batch"], root, {
      input: list.map((entry) => entry.oid).join("\n") + "\n", encoding: null,
    });
    let at = 0;
    return list.map((entry) => {
      const end = output.indexOf(10, at);
      const header = output.subarray(at, end).toString("ascii").split(" ");
      const length = Number(header[2]);
      if (header[1] !== "blob" || !Number.isSafeInteger(length)) throw new Error("invalid-blob");
      at = end + 1;
      const content = output.subarray(at, at + length);
      at += length + 1;
      return { ...entry, content };
    });
  }
  return list.map((entry) => {
    const path = resolve(root, entry.path);
    const content = lstatSync(path).isSymbolicLink() ? Buffer.from("unsupported symlink") : readFileSync(path);
    return { ...entry, content };
  });
}
export const main = (url) => process.argv[1] && url === pathToFileURL(resolve(process.argv[1])).href;
export function run(action) {
  try {
    action();
  } catch {
    console.error("repository [check-error]");
    process.exitCode = 1;
  }
}
export function finish(findings) {
  for (const { path, category } of findings) console.error(`${path} [${category}]`);
  if (findings.length) process.exitCode = 1;
}
