// Source: docklet-next/issues/469
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M38 check that the Settings › Updates Mode help explains both modes.

import { describe, expect, it } from "vitest";
import { deCommon } from "@/shared/messages/de";
import { enCommon } from "@/shared/messages/en";
import { deSettings } from "./messages/de";
import { enSettings } from "./messages/en";

describe("#469 Mode column help", () => {
  it.each([["en", enSettings, enCommon], ["de", deSettings, deCommon]] as const)("names and explains both modes (%s)", (_locale, settings, common) => {
    const help = settings.updates.modeHelp;
    expect([common.stackUpdates.checkOnly, common.stackUpdates.updateWithCheck].every((mode) => help.includes(mode))).toBe(true);
    expect(help === common.stackUpdates.updateWithCheckHelp).toBe(false);
  });
});
