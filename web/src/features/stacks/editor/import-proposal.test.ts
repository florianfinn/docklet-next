// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original behavioural break-map proofs. M19 web consumer integration.

import { expect, it } from "vitest";
import { parse } from "yaml";
import { applyImportProposal } from "./import-proposal";
const proposal = { title: "example-app", composeContent: "services:\n  app:\n    image: example/app:1\nx-custom: preserved\n", envNames: ["TOKEN", "EXISTING", "TOKEN"], found: [], missing: [] };
it("apply replaces compose, fills an empty name and adds empty env names without overwriting values", () => {
  const applied = applyImportProposal(proposal, "", "EXISTING=keep\n");
  expect(parse(applied.compose)).toEqual({ services: { app: { image: "example/app:1" } }, "x-custom": "preserved", name: "example-app" });
  expect(applied.env).toBe("EXISTING=keep\nTOKEN=\n");
  expect(parse(applyImportProposal(proposal, "my-stack", "").compose).name).toBe("my-stack");
});
