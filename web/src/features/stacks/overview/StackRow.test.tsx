import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HOSTS } from "@/data/demo/fixtures/hosts";
import { STACKS } from "@/data/demo/fixtures/stacks";
import { renderWithApp } from "@/test/render";
import { Table, TableBody } from "@/shared/ui/table";

import { StackRow } from "./StackRow";

const stack = STACKS.find((s) => s.id === "media");
const host = HOSTS.find((h) => h.id === "atlas");
const columns = ["name", "status", "host", "services", "updates", "load"].map((id) => ({ id, label: id }));

function setup(onSelect = vi.fn()) {
  if (!stack) throw new Error("fixture missing");
  const view = renderWithApp(
    <Table>
      <TableBody>
        <StackRow stack={stack} host={host} mode="compact" view="table" columns={columns} selecting selected={false} onSelect={onSelect} memLimitMb={2048} />
      </TableBody>
    </Table>,
  );
  const row = screen.getByRole("row");
  return { ...view, row, onSelect };
}

describe("stack row navigation", () => {
  it("navigates when the row background is clicked", async () => {
    const { row, router } = setup();
    const cells = within(row).getAllByRole("cell");
    await userEvent.click(cells[2] as HTMLElement);
    expect(router.state.location.pathname).toBe("/stacks/media");
  });

  it("offers a real link for keyboard and assistive technology", () => {
    const { row } = setup();
    const link = within(row).getByRole("link", { name: /Media/ });
    expect(link.getAttribute("href") === "/stacks/media").toBe(true);
  });

  it("does not navigate when the selection checkbox is used", async () => {
    const { row, router, onSelect } = setup();
    await userEvent.click(within(row).getByRole("checkbox"));
    expect(onSelect).toHaveBeenCalledWith("media", true);
    expect(router.state.location.pathname).toBe("/");
  });

  it("opens the update hover card instead of navigating, also by keyboard", async () => {
    const { row, router } = setup();
    const trigger = within(row).getByRole("button", { name: /Update-Status für Media/ });
    await userEvent.click(trigger);
    expect(router.state.location.pathname).toBe("/");
    const dialog = await screen.findByRole("dialog");
    // Clicking inside portalled content bubbles through React but must not navigate.
    await userEvent.click(within(dialog).getByText(/Updates verfügbar/));
    expect(router.state.location.pathname).toBe("/");
    expect(within(dialog).getByRole("button", { name: "Stack aktualisieren" }) instanceof HTMLButtonElement).toBe(true);
  });

  it("opens the actions menu without navigating", async () => {
    const { row, router } = setup();
    await userEvent.click(within(row).getByRole("button", { name: /Weitere Aktionen: Media/ }));
    expect(router.state.location.pathname).toBe("/");
    expect((await screen.findByRole("menu")) instanceof HTMLElement).toBe(true);
  });
});

describe("stack card", () => {
  it("shows every column as a labelled field, keeps selection and navigates from the card", async () => {
    if (!stack) throw new Error("fixture missing");
    const onSelect = vi.fn();
    const advanced = [...columns, { id: "ports", label: "Ports" }, { id: "auto", label: "Auto-Update" }, { id: "deploy", label: "Letzter Deploy" }];
    const { router } = renderWithApp(
      <ul>
        <StackRow stack={stack} host={host} mode="advanced" view="cards" columns={advanced} selecting selected={false} onSelect={onSelect} memLimitMb={2048} />
      </ul>,
    );
    const card = screen.getByRole("listitem");
    // Every non-name column becomes a term; nothing of Advanced is dropped.
    expect(within(card).getAllByRole("term").map((el) => el.textContent)).toEqual(["status", "host", "services", "updates", "load", "Ports", "Auto-Update", "Letzter Deploy"]);
    await userEvent.click(within(card).getByRole("checkbox", { name: "Media auswählen" }));
    expect(onSelect).toHaveBeenCalledWith("media", true);
    expect(router.state.location.pathname).toBe("/");
    await userEvent.click(within(card).getByText("Ports"));
    expect(router.state.location.pathname).toBe("/stacks/media");
  });
});
