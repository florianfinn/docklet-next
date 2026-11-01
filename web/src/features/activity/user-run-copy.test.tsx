// Source: docklet-next/issues/323
// Revision: 2cec9701d8c73f3185eef046b6f890dc79c6f0f6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original user run-copy guard (M15 smoke DEF-2).

import { render, screen } from "@testing-library/react";
import { IntlProvider } from "use-intl";
import { expect, it } from "vitest";

import { messages } from "@/app/i18n/messages";
import { ActionToastView } from "./ActionToastView";
import { sampleRun } from "./sample-run";

it.each([false, true])("labels a minor toast by what it opens (opensDialog=%s)", (opensDialog) => {
  const run = sampleRun(new Date().toISOString());
  render(
    <IntlProvider locale="de" messages={messages} timeZone="Europe/Berlin">
      <ActionToastView run={run} design="bar" detail="full" now={Date.now()} major={false} opensDialog={opensDialog} onClose={() => {}} onOpen={() => {}} />
    </IntlProvider>,
  );
  const label = document.querySelector('[data-slot="toast-open"]')?.getAttribute("aria-label") ?? "";
  expect(label.startsWith(opensDialog ? "Ablauf öffnen" : "Im Aktionszentrum öffnen")).toBe(true);
  expect(screen.queryAllByText(run.title).length > 0).toBe(true);
});
