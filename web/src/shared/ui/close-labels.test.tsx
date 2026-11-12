// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original regression for translated dialog and sheet close labels (M16 candidate F04).

import { screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { renderWithApp } from "@/test/render";

import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogTitle } from "./dialog";
import { Sheet, SheetContent, SheetDescription, SheetTitle } from "./sheet";

function Overlay({ kind }: { kind: "dialog" | "sheet" }) {
  return kind === "dialog"
    ? <Dialog open><DialogContent><DialogTitle>Ziel</DialogTitle><DialogDescription>Ziel bearbeiten</DialogDescription><DialogFooter showCloseButton /></DialogContent></Dialog>
    : <Sheet open><SheetContent><SheetTitle>Dateien</SheetTitle><SheetDescription>Ziel bearbeiten</SheetDescription></SheetContent></Sheet>;
}

afterEach(() => { setAppLocale("de"); });

describe("dialog and sheet close labels", () => {
  // The dialog has its corner and footer close controls, the sheet its corner control.
  it.each([["dialog", 2], ["sheet", 1]] as const)("names every %s close control in German", async (kind, count) => {
    setAppLocale("de");
    renderWithApp(<Overlay kind={kind} />);
    await screen.findByText("Ziel bearbeiten");
    expect(screen.queryAllByRole("button", { name: "Schließen" }).length).toBe(count);
    expect(screen.queryAllByRole("button", { name: "Close" }).length).toBe(0);
  });
  it.each([["dialog", 2], ["sheet", 1]] as const)("names every %s close control in English", async (kind, count) => {
    setAppLocale("en-US");
    renderWithApp(<Overlay kind={kind} />);
    await screen.findByText("Ziel bearbeiten");
    expect(screen.queryAllByRole("button", { name: "Close" }).length).toBe(count);
  });
});
