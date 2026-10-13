import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { OriginBadge } from "@/features/containers/OriginBadge";
import { renderWithApp } from "@/test/render";
import { Input } from "@/shared/ui/input";

import { DEFAULT_HELP, helpStore, mergeHelp, setHelpMode } from "@/shared/preferences/help-store";
import { HelpLabel, HelpTarget } from "./help-tooltip";
import { deHelp } from "./messages/de";

afterEach(() => {
  helpStore.reset();
  localStorage.clear();
});

function Form() {
  return (
    <form>
      <HelpLabel htmlFor="addr" topic="settingsGeneral.hubAddress">
        Interne Hub-Adresse
      </HelpLabel>
      <Input id="addr" />
      <HelpTarget topic="action.restart">
        <button type="button">Neustart</button>
      </HelpTarget>
      <OriginBadge origin="unraid" />
    </form>
  );
}

const trigger = () => screen.queryByRole("button", { name: "Erklärung: Interne Hub-Adresse" });
const tooltipText = async () => (await screen.findByRole("tooltip")).textContent ?? "";

describe("extended explanations", () => {
  it("defaults to full and keeps unknown stored values out", () => {
    expect(DEFAULT_HELP.mode).toBe("full");
    expect(mergeHelp({ mode: "loud" }, DEFAULT_HELP).mode).toBe("full");
    expect(mergeHelp({ mode: "off", extra: 1 }, DEFAULT_HELP)).toEqual({ mode: "off" });
  });

  it("full mode explains title, text and impact on keyboard focus and closes with Escape", async () => {
    renderWithApp(<Form />);
    // The info button sits beside the label, so the input keeps its plain name.
    expect(screen.getByLabelText("Interne Hub-Adresse").id).toBe("addr");
    await userEvent.tab();
    expect(document.activeElement === trigger()).toBe(true);
    const text = await tooltipText();
    expect(text.includes("Edge-Agents bauen ihre Verbindung") && text.includes("Auswirkung:")).toBe(true);
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("tooltip")).toBe(null));
    expect(document.activeElement === trigger()).toBe(true);
  });

  it("stays closed when focus arrives without being visible (a dialog focusing its first field), but a click opens it", async () => {
    // happy-dom treats every focus as visible; stub the browser's answer for programmatic focus after a click.
    const real = HTMLElement.prototype.matches;
    const spy = vi.spyOn(HTMLElement.prototype, "matches").mockImplementation(function (this: HTMLElement, selector: string) {
      return selector === ":focus-visible" || selector === ":hover" ? false : real.call(this, selector);
    });
    renderWithApp(<Form />);
    act(() => trigger()!.focus());
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("tooltip")).toBe(null);
    await userEvent.click(trigger()!);
    expect((await tooltipText()).includes("Edge-Agents")).toBe(true);
    spy.mockRestore();
  });

  it("opens on hover after a delay, toggles on click and explains actions on their own button", async () => {
    renderWithApp(<Form />);
    await userEvent.hover(trigger()!);
    expect(screen.queryByRole("tooltip")).toBe(null);
    await screen.findByRole("tooltip", {}, { timeout: 1500 });
    // Pointer geometry is not laid out under happy-dom; Escape stands in for leaving.
    await userEvent.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("tooltip")).toBe(null));

    await userEvent.click(trigger()!);
    expect((await tooltipText()).includes("Interne Hub-Adresse")).toBe(true);
    await userEvent.click(trigger()!);
    await waitFor(() => expect(screen.queryByRole("tooltip")).toBe(null));

    await userEvent.click(document.body);
    await userEvent.tab();
    await userEvent.tab();
    await userEvent.tab();
    expect(document.activeElement?.textContent).toBe("Neustart");
    await waitFor(() => expect(screen.getAllByRole("tooltip").map((t) => t.textContent ?? "").join(" | ")).toMatch(/^Neustart.*Änderungen an compose.yaml oder .env werden dabei nicht übernommen/));
  });

  it("compact mode shows one sentence without title or impact", async () => {
    setHelpMode("compact");
    renderWithApp(<Form />);
    await userEvent.click(trigger()!);
    expect(await tooltipText()).toBe(deHelp.topics.settingsGeneral.hubAddress.short);
  });

  it("off hides only the extended explanations; the provenance hover stays", async () => {
    setHelpMode("off");
    renderWithApp(<Form />);
    expect(trigger()).toBe(null);
    expect(screen.getByRole("button", { name: "Neustart" }).hasAttribute("data-state")).toBe(false);
    await userEvent.hover(screen.getByText("Unraid"));
    expect((await screen.findByText("Von Unraid verwaltet", {}, { timeout: 1500 })) !== null).toBe(true);
  });

  it("has a title, short text, full text and impact for every topic", () => {
    const topics = Object.entries(deHelp.topics).flatMap(([g, items]) => Object.entries(items).map(([k, v]) => [`${g}.${k}`, v] as const));
    expect(topics.length > 70).toBe(true);
    const incomplete = topics.filter(([, v]) => !(v.title && v.short && v.full && v.impact)).map(([k]) => k);
    expect(incomplete).toEqual([]);
    // ICU: braces and single apostrophes would break formatting; texts use none.
    expect(topics.filter(([, v]) => /[{}']/.test(Object.values(v).join(" "))).map(([k]) => k)).toEqual([]);
  });
});
