import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import { DEFAULT_HELP, helpStore, mergeHelp } from "@/shared/preferences/help-store";
import { renderWithApp } from "@/test/render";

import { HelpSection } from "./sections/HelpSection";

afterEach(() => {
  helpStore.reset();
  localStorage.clear();
});

const KEY = "docklet-react-opus.help";
const mode = () => within(screen.getByRole("radiogroup", { name: "Erklärungen" }));

describe("explanation settings", () => {
  it("keeps the mode as a draft until Save, discards it and stores it in this browser", async () => {
    renderWithApp(<HelpSection />);
    expect(mode().getByRole("radio", { name: "Ausführlich" }).getAttribute("aria-checked")).toBe("true");
    expect(screen.getByText(/^Wird in diesem Browser gespeichert/) !== null).toBe(true);

    await userEvent.click(mode().getByRole("radio", { name: "Kurz" }));
    expect(screen.getByRole("status").textContent).toBe("Ungespeicherte Änderungen");
    // The live preview follows the draft; the saved mode is untouched.
    expect(screen.getByText("Adresse, unter der Agents den Hub erreichen.") !== null && helpStore.get().mode === "full").toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Verwerfen" }));
    expect(mode().getByRole("radio", { name: "Ausführlich" }).getAttribute("aria-checked")).toBe("true");

    await userEvent.click(mode().getByRole("radio", { name: "Aus" }));
    expect(screen.getByText("Info-Symbole sind ausgeblendet.") !== null).toBe(true);
    expect(screen.queryByRole("button", { name: "Erklärung: Erklärungen" }) !== null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
    await waitFor(() => expect(screen.getByRole("status").textContent).toBe("Alles gespeichert"));
    expect(helpStore.get().mode).toBe("off");
    // After saving "off" the section's own info button is gone as well.
    expect(screen.queryByRole("button", { name: "Erklärung: Erklärungen" })).toBe(null);

    // A reload reads the stored value back.
    const stored = JSON.parse(localStorage.getItem(KEY) ?? "null") as { v: number; data: unknown };
    expect(stored.v === 1 && mergeHelp(stored.data, DEFAULT_HELP).mode === "off").toBe(true);
  });
});
