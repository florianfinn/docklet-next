// Source: docklet-next/issues/217
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original volume.exists web contract tests for create and rename.

import { describe, expect, it } from "vitest";
import { AdapterError } from "@/data/api";
import { volumeFixture } from "./volume-commands.test-support";

describe.each([false, true])("volume name conflicts (Edge=%s)", (edge) => {
  it.each(["create", "rename"])("maps volume.exists on %s to the existing translated exists error", async (operation) => {
    const f = volumeFixture(edge); f.state.status = 409; f.state.code = "volume.exists";
    const pending = operation === "create" ? f.adapter.createVolume({ hostId: "host", name: "existing", labels: {} }) : f.adapter.renameVolume("host~data", "existing");
    await expect(pending).rejects.toBeInstanceOf(AdapterError);
    await expect(pending).rejects.toMatchObject({ code: "exists" });
    expect(f.state.calls).toHaveLength(1);
  });

  it("leaves unknown volume conflict codes on the generic outcome path", async () => {
    const f = volumeFixture(edge); f.state.status = 409; f.state.code = "volume.future-conflict";
    await expect(f.adapter.renameVolume("host~data", "existing")).rejects.toMatchObject({ outcome: { kind: "error", code: "volume.future-conflict", status: 409 } });
    expect(f.state.calls).toHaveLength(1);
  });
});
