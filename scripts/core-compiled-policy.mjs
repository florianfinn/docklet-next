// Exact compiled header evidence selected for the linux/amd64 SQLite product.
const pins = new Map([
  ["sys/types/types_linux_amd64.go", "1ada8b2599b13281b8fe191cfb24c02896ec3781b95a3b5cf76e1d74424eea2e"],
  ["uuid/uuid/uuid_linux_amd64.go", "c677ffac9af69102fabc82443b8799b4071c7ead9f02237bb2f01af7782af428"],
]);
export function checkCompiledNotices(manifest) {
  if (manifest.target !== "linux/amd64" || manifest.cgo !== "0" || manifest.files?.length !== pins.size)
    throw new Error("compiled-licence-scope");
  const seen = new Set();
  for (const row of manifest.files) {
    if (row.module !== "modernc.org/libc" || row.version !== "v1.77.1" ||
        pins.get(row.path) !== row.sha256 || !row.reason?.includes("section 5") ||
        JSON.stringify(row.targets) !== JSON.stringify(["hub", "agent"]) || seen.has(row.path))
      throw new Error("compiled-licence-evidence");
    seen.add(row.path);
  }
}
