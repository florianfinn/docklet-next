// Source: docklet-next/issues/520
// Revision: 462df7e945082b44e1d90532d8296aa9743c4b6e
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original IP address/domain field and notification link choice tests.

import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";

import { notificationBaseUrl, type GeneralSettings, type LinkBase } from "@/domain/settings";
import { freshSetupDraft } from "@/features/setup/setup-draft";
import { renderWithApp } from "@/test/render";

import { GeneralFields } from "./GeneralSection";

afterEach(() => { cleanup(); localStorage.clear(); });

const writes: Partial<GeneralSettings>[] = [];
function Fields({ initial }: { initial: Partial<GeneralSettings> }) {
  const [value, setValue] = useState<GeneralSettings>({ ...freshSetupDraft().general, ...initial });
  return <GeneralFields value={value} onChange={(p) => { writes.push(p); setValue((v) => ({ ...v, ...p })); }} showErrors={false} showLanguage={false} />;
}

const radio = (name: string) => screen.getByRole("radio", { name }) as HTMLButtonElement;
const checked = (name: string) => radio(name).getAttribute("aria-checked") === "true";

it("offers only the IP address until a domain is set, and says why", async () => {
  renderWithApp(<Fields initial={{ hubAddress: "192.0.2.10:8080", domainUrl: "", linkBase: "auto" }} />);
  expect(await screen.findByLabelText("IP-Adresse") !== null).toBe(true);
  expect([radio("Über die Domain").disabled, checked("Über die IP-Adresse")]).toEqual([true, true]);
  expect(screen.getByText("Erst eine Domain eintragen").id).toBe(radio("Über die Domain").getAttribute("aria-describedby"));
  await userEvent.type(screen.getByLabelText("Domain"), "https://hub.example.com");
  expect([radio("Über die Domain").disabled, checked("Über die Domain")]).toEqual([false, true]);
  expect(screen.queryByText("Erst eine Domain eintragen") === null).toBe(true);
  writes.length = 0;
  await userEvent.click(radio("Über die IP-Adresse"));
  expect(writes).toEqual([{ linkBase: "internal" }]);
  expect(checked("Über die IP-Adresse")).toBe(true);
});

// The shown choice is the backend's resolution (resolveLinkBase): auto and domain use a set domain.
it.each([
  ["auto", "", "internal"], ["auto", "https://hub.example.com", "domain"],
  ["domain", "", "internal"], ["domain", "https://hub.example.com", "domain"],
  ["internal", "", "internal"], ["internal", "https://hub.example.com", "internal"],
] as const)("stored %s with domain %j shows %s", async (linkBase, domainUrl, shown) => {
  renderWithApp(<Fields initial={{ hubAddress: "192.0.2.10:8080", domainUrl, linkBase: linkBase as LinkBase }} />);
  await screen.findByLabelText("IP-Adresse");
  expect(checked(shown === "domain" ? "Über die Domain" : "Über die IP-Adresse")).toBe(true);
  expect(notificationBaseUrl({ ...freshSetupDraft().general, hubAddress: "192.0.2.10:8080", domainUrl, linkBase: linkBase as LinkBase }).source).toBe(shown);
});

it("keeps the time zone link button to its text width", async () => {
  renderWithApp(<Fields initial={{}} />);
  const button = await screen.findByRole("button", { name: /^TZ verwenden|^Use TZ/ });
  expect(button.className.split(" ").includes("w-fit")).toBe(true);
  // Field gives its direct children full width ([&>*]:w-full); the link must sit inside a wrapper.
  expect(button.parentElement?.getAttribute("data-slot") === "field").toBe(false);
});
