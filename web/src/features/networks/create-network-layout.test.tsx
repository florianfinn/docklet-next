import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";

import { HOSTS } from "@/data/demo/fixtures/hosts";
import { renderWithApp } from "@/test/render";

import { CreateNetworkDialog } from "./CreateNetworkDialog";

const section = (family: string) => document.querySelector<HTMLElement>(`[data-family="${family}"]`)!;
const fields = (family: string) => [...section(family).querySelectorAll("input:not([role=switch])")].map((i) => i.id);

describe("network wizard layout", () => {
  it("keeps name, both family switches and the shared hint in place; a family's fields appear only below the switch row", async () => {
    renderWithApp(<CreateNetworkDialog open onOpenChange={() => undefined} hosts={HOSTS} networks={[]} defaultHostId="atlas" onCreated={() => undefined} />);
    const dialog = await screen.findByRole("dialog", { name: "Netzwerk anlegen" });
    // Size is fixed from the start; the body is the only scroll region.
    expect([dialog.className.includes("h-[min(56rem,calc(100dvh-2rem))]"), dialog.querySelectorAll('[data-slot="wizard-body"]').length]).toEqual([true, 1]);
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    const order = () => [...dialog.querySelectorAll('#net-name, #net-ipv4, #net-ipv6, [data-family], #net-families-hint, [data-slot="family-error"]')].map((e) => e.getAttribute("data-family") || e.id || e.getAttribute("data-slot"));
    const before = order();
    // Both switches share one row above every expandable field, never inside a family section.
    const row = dialog.querySelector('[data-slot="family-switches"]')!;
    expect([before.slice(-4), row.contains(dialog.querySelector("#net-ipv4")), row.contains(dialog.querySelector("#net-ipv6")), section("ipv4").querySelector("[role=switch]") === null]).toEqual([["net-ipv4", "net-ipv6", "ipv4", "ipv6"], true, true, true]);
    expect([fields("ipv4"), fields("ipv6")]).toEqual([["net-subnet", "net-gateway", "net-range"], []]);

    await userEvent.click(within(dialog).getByRole("switch", { name: "IPv6" }));
    expect([fields("ipv4"), fields("ipv6")]).toEqual([
      ["net-subnet", "net-gateway", "net-range"],
      ["net-subnet6", "net-gateway6", "net-range6"],
    ]);
    await userEvent.click(within(dialog).getByRole("switch", { name: "IPv4" }));
    expect([fields("ipv4"), fields("ipv6"), section("ipv4").textContent?.includes("keine IPv4-Adressen")]).toEqual([[], ["net-subnet6", "net-gateway6", "net-range6"], true]);
    // Both anchors, the shared hint and the reserved error line never move in the document order.
    expect(order()).toEqual(before);

    await userEvent.click(within(dialog).getByRole("switch", { name: "IPv6" }));
    await userEvent.type(within(dialog).getByLabelText("Name"), "lan_test");
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    expect([order(), dialog.querySelector('[data-slot="family-error"]')?.textContent]).toEqual([before, "Mindestens IPv4 oder IPv6 einschalten"]);
  });
});
