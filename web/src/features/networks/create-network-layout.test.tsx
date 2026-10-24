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

  it("keeps advanced fields collapsed, validates duplicate keys before conversion and preserves the family anchors", async () => {
    renderWithApp(<CreateNetworkDialog open onOpenChange={() => undefined} hosts={HOSTS} networks={[]} defaultHostId="atlas" onCreated={() => undefined} />);
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    await userEvent.type(within(dialog).getByLabelText("Name"), "example_net");
    const advanced = within(dialog).getByRole("button", { name: "Erweitert" });
    expect(advanced.getAttribute("aria-expanded")).toBe("false");
    expect(within(dialog).queryByRole("button", { name: "Hinzufügen: Labels" }) === null).toBe(true);
    await userEvent.click(advanced);
    await userEvent.click(within(dialog).getByRole("button", { name: "Hinzufügen: Labels" }));
    let labels = within(dialog).getByRole("group", { name: "Labels" });
    await userEvent.type(within(labels).getByLabelText("Schlüssel"), "example.com/team");
    await userEvent.type(within(labels).getByLabelText("Wert"), "web");
    await userEvent.click(within(labels).getByRole("button", { name: "Hinzufügen: Labels" }));
    await userEvent.type(within(labels).getAllByLabelText("Schlüssel")[1]!, "example.com/team");
    await userEvent.click(advanced);
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    // A failed submit reopens Advanced, so errors cannot remain hidden.
    labels = within(dialog).getByRole("group", { name: "Labels" });
    expect(advanced.getAttribute("aria-expanded")).toBe("true");
    expect(within(labels).getAllByText("Schlüssel, Namen und Adressen müssen eindeutig sein").length > 0).toBe(true);
    await userEvent.click(within(labels).getByRole("button", { name: "Eintrag 2 in Labels entfernen" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Hinzufügen: Treiberoptionen" }));
    const options = within(dialog).getByRole("group", { name: "Treiberoptionen" });
    await userEvent.type(within(options).getByLabelText("Schlüssel"), "com.docker.network.driver.mtu");
    await userEvent.type(within(options).getByLabelText("Wert"), "1400");
    await userEvent.type(within(dialog).getByLabelText("IPv4-Subnetz (CIDR)"), "192.0.2.0/24");
    await userEvent.click(within(dialog).getByRole("button", { name: "Hinzufügen: IPv4-Hilfsadressen" }));
    const aux = within(dialog).getByRole("group", { name: "IPv4-Hilfsadressen" });
    await userEvent.type(within(aux).getByLabelText("Reservierungsname"), "reserved");
    await userEvent.type(within(aux).getByLabelText("Reservierte IP-Adresse"), "192.0.2.2");
    await userEvent.click(within(dialog).getByRole("switch", { name: "IPv6" }));
    expect(within(dialog).getByRole("button", { name: "Hinzufügen: IPv6-Hilfsadressen" }) !== null).toBe(true);
    await userEvent.click(within(dialog).getByRole("switch", { name: "IPv4" }));
    expect(within(dialog).queryByRole("button", { name: "Hinzufügen: IPv4-Hilfsadressen" }) === null).toBe(true);
    await userEvent.click(within(dialog).getByRole("switch", { name: "IPv4" }));
    expect((within(within(dialog).getByRole("group", { name: "IPv4-Hilfsadressen" })).getByLabelText("Reservierte IP-Adresse") as HTMLInputElement).value).toBe("192.0.2.2");
    expect([fields("ipv4"), fields("ipv6")]).toEqual([["net-subnet", "net-gateway", "net-range"], ["net-subnet6", "net-gateway6", "net-range6"]]);
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    expect(within(dialog).getByText("example.com/team=web") !== null).toBe(true);
    expect(within(dialog).getByText("com.docker.network.driver.mtu=1400") !== null).toBe(true);
    expect(within(dialog).getByText("reserved=192.0.2.2") !== null).toBe(true);
  });
