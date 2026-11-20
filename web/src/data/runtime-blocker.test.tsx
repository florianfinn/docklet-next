// Source: docklet-next/issues/465
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original runtime blocker rendering through the HTTP error mapper.

import { render, screen } from "@testing-library/react";
import { IntlProvider } from "use-intl";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { runtimeResponseError } from "./http/runtime-mapping";
import { useCommandError } from "./use-command-error";
import { useImageError } from "@/features/images/use-image-error";
import { useRuntimeError } from "@/features/activity/use-runtime-error";

function Refusal({ error }: { error: unknown }) {
 const command = useCommandError(), image = useImageError();
 return <><p role="alert">{command(error)}</p><p>{image(error)}</p></>;
}

it.each(["en-US", "de"] as const)("names an archived blocking stack and its reason in %s", (locale) => {
 const error = runtimeResponseError({success:false,error:{code:"runtime.target-unavailable",message:"private diagnostic",blocker:{name:"example-peer",reason:"compose.selection-unsupported",archived:true}}},409);
 const view = render(<IntlProvider locale={locale} messages={catalogs[locale]}><Refusal error={error}/></IntlProvider>);
 const text = screen.getByRole("alert").textContent ?? "";
 expect(text).toContain("example-peer");
 expect(text).toMatch(locale === "de" ? /archiviert/i : /archived/i);
 expect(text).toMatch(locale === "de" ? /Compose-Auswahl/i : /Compose selection/i);
 expect(view.container.textContent).not.toContain("private diagnostic");
 expect(view.container.querySelectorAll("p")[1]?.textContent).toBe(text);
});

// Stack and container command guards ask the runtime formatter first and fall back to the command formatter.
function GuardedRefusal({ error }: { error: unknown }) {
 const runtime = useRuntimeError(), command = useCommandError();
 const reported: string[] = [];
 if (!runtime(error, (message) => { reported.push(message); return ""; })) reported.push(command(error));
 return <p role="alert">{reported.join(" ")}</p>;
}

it.each(["en-US", "de"] as const)("names the blocking stack through the command guard order in %s", (locale) => {
 const error = runtimeResponseError({success:false,error:{code:"runtime.target-unavailable",message:"private diagnostic",blocker:{name:"example-peer",reason:"compose.identity-unreadable",archived:false}}},409);
 render(<IntlProvider locale={locale} messages={catalogs[locale]}><GuardedRefusal error={error}/></IntlProvider>);
 const text = screen.getByRole("alert").textContent ?? "";
 expect(text).toContain("example-peer");
 expect(text).not.toContain("private diagnostic");
});

it.each([
 {code:"runtime.target-unavailable",blocker:{name:"/srv/private",reason:"compose.selection-unsupported",archived:false}},
 {code:"runtime.target-unavailable",blocker:{name:"example-peer",reason:"arbitrary.diagnostic",archived:false}},
 {code:"runtime.target-changed",blocker:{name:"example-peer",reason:"compose.selection-unsupported",archived:false}},
])("does not render an invalid or unrelated blocker %#", (problem) => {
 const view = render(<IntlProvider locale="en-US" messages={catalogs["en-US"]}><Refusal error={runtimeResponseError({success:false,error:problem},409)}/></IntlProvider>);
 expect(view.container.textContent).not.toContain(problem.blocker.name);
 expect(view.container.textContent).not.toContain(problem.blocker.reason);
});

it.each(["en-US", "de"] as const)("renders invalid confirmations through the command error consumer in %s", (locale) => {
 const error = runtimeResponseError({ success: false, error: { code: "runtime.invalid-acknowledgement" } }, 400);
 const view = render(<IntlProvider locale={locale} messages={catalogs[locale]}><Refusal error={error}/></IntlProvider>);
 expect(view.container.textContent).toContain(catalogs[locale].common.runtimePeers.invalid);
 expect(view.container.textContent).not.toContain("runtime.invalid-acknowledgement");
 view.unmount();
});
