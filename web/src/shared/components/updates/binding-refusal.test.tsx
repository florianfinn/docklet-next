// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original total bilingual binding render proof.

import { render } from "@testing-library/react";
import { IntlProvider } from "use-intl";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { IMAGE_BINDING_KEYS } from "@/data/wire-catalogue-update";
import type { ImageBinding } from "@/data/http/update-binding";
import { BindingRefusal } from "./binding-refusal";
it.each(["en-US", "de"] as const)("renders every binding reason in %s with service and variable detail", (locale) => {
  for (const reason of Object.keys(IMAGE_BINDING_KEYS) as ImageBinding["reason"][]) {
    const binding: ImageBinding = { scope: "service", service: "example-service", reason, ...(reason === "variable-reference" ? { variable: "SERVICE_IMAGE" } : {}) };
    const view = render(<IntlProvider locale={locale} messages={catalogs[locale]}><BindingRefusal binding={binding} /></IntlProvider>);
    const text = view.container.textContent ?? "";
    expect(text.includes("images.errors.")).toBe(false);
    expect(text).toContain(locale === "de" ? "Update nicht angeboten:" : "Update not offered:");
    expect(text).toContain("example-service");
    if (reason === "variable-reference") expect(text).toContain("SERVICE_IMAGE");
    view.unmount();
  }
});
