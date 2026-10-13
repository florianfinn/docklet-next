import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { describe, expect, it } from "vitest";

import { parseCompose } from "@/domain/compose/model";
import { mutate } from "@/domain/compose/ops";
import { validateCompose } from "@/domain/compose/validate";
import { renderWithApp } from "@/test/render";

import { HealthSection } from "./ServiceSections";

let latest = "";

function Harness({ initial }: { initial: string }) {
  const [text, setText] = useState(initial);
  useEffect(() => {
    latest = text;
  }, [text]);
  const parsed = parseCompose(text);
  if (!parsed.ok) return null;
  const service = parsed.model.services[0]!;
  return <HealthSection service={service} issues={validateCompose(parsed.model)} edit={(fn) => setText((t) => mutate(t, fn))} />;
}

const NO_HEALTH = `services:\n  cache:\n    image: redis:7.4-alpine\n`;
const MANUAL = `services:\n  web:\n    image: nginx:1.29\n    ports:\n      - "8080:80"\n    healthcheck:\n      test: ["CMD-SHELL", "curl -f http://localhost/ping"]\n      interval: 10s\n`;

describe("healthcheck presets in the Easy editor", () => {
  it("fills the matching preset only on an explicit click", async () => {
    renderWithApp(<Harness initial={NO_HEALTH} />);
    const presets = screen.getByRole("region", { name: "Vorschläge" });
    expect(within(presets).getByText("Braucht im Image: redis-cli") !== null).toBe(true);
    expect(latest.includes("healthcheck")).toBe(false);
    await userEvent.click(within(presets).getByRole("button", { name: "Vorschlag „Redis antwortet (redis-cli ping)“ übernehmen" }));
    expect([latest.includes("test: [ CMD, redis-cli, ping ]"), (screen.getByLabelText("Testbefehl als Argumentliste") as HTMLInputElement).value]).toEqual([true, '["redis-cli", "ping"]']);
    expect(screen.getByText("Direkt (CMD)") !== null).toBe(true);
  });

  it("asks before replacing a hand-written test; cancel keeps it, confirm replaces it", async () => {
    renderWithApp(<Harness initial={MANUAL} />);
    const presets = screen.getByRole("region", { name: "Vorschläge" });
    // Unknown image: generic HTTP checks, with the port taken from the service and the tool named.
    expect(within(presets).getByText("Braucht im Image: curl") !== null && within(presets).getAllByText(/Port 80 \(erster Container-Port\)/).length).toBe(2);
    await userEvent.click(within(presets).getByRole("button", { name: "Vorschlag „HTTP-Antwort mit wget“ übernehmen" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Vorhandenen Healthcheck ersetzen?" });
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    expect(latest.includes("curl -f http://localhost/ping")).toBe(true);

    await userEvent.click(within(presets).getByRole("button", { name: "Vorschlag „HTTP-Antwort mit wget“ übernehmen" }));
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Ersetzen" }));
    expect([latest.includes("wget -q --spider http://localhost:80/ || exit 1"), latest.includes("curl -f")]).toEqual([true, false]);
  });

  it("edits CMD as an exact JSON argument list and never writes invalid JSON", async () => {
    const exec = MANUAL.replace('["CMD-SHELL", "curl -f http://localhost/ping"]', '["CMD", "probe", "--name", "a b"]\n      # bleibt\n      x-extra: 1');
    renderWithApp(<Harness initial={exec} />);
    const field = screen.getByLabelText("Testbefehl als Argumentliste") as HTMLInputElement;
    expect(field.value).toBe('["probe", "--name", "a b"]');
    const args = () => (parseCompose(latest) as { ok: true; model: { services: { health: { args: string[] } }[] } }).model.services[0]!.health.args;

    // A half-typed list shows the error and leaves the file as it was.
    await userEvent.clear(field);
    await userEvent.type(field, '[[ "tool", "--label"');
    expect([screen.getByText(/Keine gültige JSON-Liste/) !== null, field.getAttribute("aria-invalid"), args()]).toEqual([true, "true", ["probe", "--name", "a b"]]);

    // Completed with spaces, an empty argument, quotes and a backslash, it is written exactly.
    await userEvent.type(field, ', "hello world", "", "say \\"hi\\"", "C:\\\\x"]');
    expect([args(), screen.queryByText(/Keine gültige JSON-Liste/), latest.includes("# bleibt"), latest.includes("x-extra: 1")]).toEqual([["tool", "--label", "hello world", "", 'say "hi"', "C:\\x"], null, true, true]);

    // The user's own spacing stays while it means the same list.
    expect(field.value).toBe('[ "tool", "--label", "hello world", "", "say \\"hi\\"", "C:\\\\x"]');
  });

  it("cancelling a preset keeps a hand-written CMD list unchanged", async () => {
    renderWithApp(<Harness initial={MANUAL.replace('["CMD-SHELL", "curl -f http://localhost/ping"]', '["CMD", "tool", "--label", "hello world", ""]')} />);
    await userEvent.click(within(screen.getByRole("region", { name: "Vorschläge" })).getByRole("button", { name: "Vorschlag „HTTP-Antwort mit curl“ übernehmen" }));
    const dialog = await screen.findByRole("alertdialog", { name: "Vorhandenen Healthcheck ersetzen?" });
    expect(within(dialog).getByText('["tool", "--label", "hello world", ""]') !== null).toBe(true);
    await userEvent.click(within(dialog).getByRole("button", { name: "Abbrechen" }));
    expect([latest.includes('"hello world", ""'), (screen.getByLabelText("Testbefehl als Argumentliste") as HTMLInputElement).value]).toEqual([true, '["tool", "--label", "hello world", ""]']);
  });

  it("shows invalid interval and retries at their own fields", async () => {
    renderWithApp(<Harness initial={MANUAL} />);
    await userEvent.clear(screen.getByLabelText("Intervall"));
    await userEvent.type(screen.getByLabelText("Intervall"), "30");
    expect(screen.getByText("Intervall ist keine Dauer wie 30s oder 1m30s") !== null).toBe(true);
    await userEvent.type(screen.getByLabelText("Versuche"), "0");
    expect(screen.getByText("Versuche müssen eine ganze Zahl ab 1 sein") !== null).toBe(true);
  });
});
