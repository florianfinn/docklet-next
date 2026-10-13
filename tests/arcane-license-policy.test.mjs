import test from "node:test";
import assert from "node:assert/strict";
import { classify, inspectLegal, inspectCompiled, blockedLicenses, assertLicense, isLegalFile, reviews, reviewFor } from "../scripts/arcane-license-policy.mjs";

const mit = "Permission is hereby granted, free of charge";
const apache = "Apache License, Version 2.0";
const bsd2 = "Redistribution and use in source and binary forms. THIS SOFTWARE";
const bsd3 = "Redistribution and use in source and binary forms. Neither the name. THIS SOFTWARE";

test("classifier collects multiple grants, including both BSD variants", () => {
  assert.equal(classify([mit, apache, bsd2, bsd3].join("\n---\n")), "Apache-2.0 AND BSD-2-Clause AND BSD-3-Clause AND MIT");
  assert.equal(classify([bsd2, "Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee"].join("\n")), "BSD-2-Clause AND ISC");
});

for (const text of ["GNU General Public License", "GNU Lesser General Public\n// License", "GNU Affero General Public License", "Server Side Public License", "Business Source License", "Commons Clause", "Elastic License", "SPDX-License-Identifier: GPL-3.0-only", "SPDX-License-Identifier: LGPL-2.1-or-later", "SPDX-License-Identifier: AGPL-3.0-only", "SPDX-License-Identifier: SSPL-1.0", "SPDX-License-Identifier: BUSL-1.1"]) {
  test(`licence guard rejects ${text.replace(/\n.*/, "")}`, () => {
    assert.ok(blockedLicenses(text).length);
    for (const path of ["LICENSE", "nested/LICENSE", "LICENSES/copyleft.txt"])
      assert.throws(() => inspectLegal("example.invalid/module", "v1.0.0", path, `${mit}\n${text}`), /blocked licence/);
    assert.throws(() => inspectCompiled("example.invalid/module", "v1.0.0", "compiled.go", `// ${text}`), /blocked compiled notice/);
  });
}

test("unclassified legal texts and unsupported SPDX terms fail closed", () => {
  assert.throws(() => inspectLegal("example.invalid/module", "v1.0.0", "nested/NOTICE", "unreviewed notice"), /unclassified/);
  for (const expression of ["Unknown", "NOASSERTION", "MIT AND GPL-3.0-only", "LGPL-2.1-or-later", "SSPL-1.0", "BUSL-1.1", "EDL-1.0", "EPL-2.0"])
    assert.throws(() => assertLicense(expression));
  assert.doesNotThrow(() => assertLicense("EPL-2.0 OR BSD-3-Clause", "BSD-3-Clause"));
});

test("review exemptions require the exact module, version, path and hash", () => {
  for (const r of reviews.filter((r) => r.allowedMarkers?.length))
    assert.throws(() => reviewFor(r.module, r.version, r.path, "changed evidence"), /evidence changed/);
  assert.throws(() => inspectCompiled("modernc.org/libc", "v1.77.2", "sys/types/types_linux_amd64.go", "// GNU Lesser General Public License"), /blocked compiled notice/);
});

test("legal discovery excludes implementation filenames and includes REUSE texts", () => {
  for (const path of ["license.go", "notice_test.go", "copyright.go", "pgproto3/notice_response.go", ".github/workflows/license-check.yml"])
    assert.equal(isLegalFile(path), false);
  for (const path of ["LICENSE", "sub/COPYING.md", "LICENSES/BSD-3-Clause.txt", "LICENSES/CC-BY-ND-4.0.txt", "sub/LICENSES/MIT.txt"])
    assert.equal(isLegalFile(path), true);
});

test("fixture subprocesses do not inherit enclosing CI/PR event state", async () => {
  const { invoke, ROOT } = await import("./helpers.mjs");
  const result = invoke(ROOT, process.execPath, ["-e", 'console.log(JSON.stringify(Object.keys(process.env).filter(k => k.startsWith("GITHUB_") || k === "CI")))']);
  assert.equal(result.status, 0);
  assert.deepEqual(JSON.parse(result.stdout), []);
  const explicit = invoke(ROOT, process.execPath, ["-e", 'console.log(process.env.GITHUB_EVENT_NAME)'], { env: { GITHUB_EVENT_NAME: "pull_request" } });
  assert.equal(explicit.stdout.trim(), "pull_request");
});
