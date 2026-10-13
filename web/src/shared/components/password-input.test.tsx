import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { DEFAULT_UI_STYLE, mergeUiStyle, setUiStyle, TOAST_SCALE, uiStyleStore, useApplyUiStyle } from "@/shared/preferences/ui-style-store";
import { renderWithApp } from "@/test/render";

import { TextField } from "./form-field";

afterEach(() => {
  uiStyleStore.reset();
  localStorage.clear();
});

function Secret() {
  const [value, setValue] = useState("");
  return <TextField id="pw" type="password" label="Passwort" value={value} onChange={setValue} error={value.length < 12 ? "Mindestens 12 Zeichen" : undefined} autoComplete="new-password" />;
}

describe("password field", () => {
  it("shows and hides the value with an aria-pressed toggle, keeps focus and the error", async () => {
    renderWithApp(<Secret />);
    const input = screen.getByLabelText("Passwort") as HTMLInputElement;
    await userEvent.type(input, "kurz");
    const toggle = screen.getByRole("button", { name: "Eingabe anzeigen" });
    expect([input.type, toggle.getAttribute("aria-pressed")]).toEqual(["password", "false"]);
    await userEvent.click(toggle);
    expect([input.type, screen.getByRole("button", { name: "Eingabe verbergen" }).getAttribute("aria-pressed"), input.value]).toEqual(["text", "true", "kurz"]);
    // A mouse click does not steal the focus; the error stays attached to the input.
    expect(document.activeElement).toBe(input);
    expect(input.getAttribute("aria-invalid") === "true" && input.getAttribute("aria-describedby") === "pw-err" && screen.getByText("Mindestens 12 Zeichen").id === "pw-err").toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Eingabe verbergen" }));
    expect(input.type).toBe("password");
    expect(JSON.stringify({ ...localStorage }).includes("kurz")).toBe(false);
  });
});

function ApplyStyle() {
  useApplyUiStyle();
  return null;
}

describe("toast size", () => {
  it("persists a bounded size and exposes it to the toasts", () => {
    renderWithApp(<ApplyStyle />);
    expect(document.documentElement.style.getPropertyValue("--toast-scale")).toBe(String(DEFAULT_UI_STYLE.toastScale));
    act(() => setUiStyle({ toastScale: 1.4 }));
    expect(document.documentElement.style.getPropertyValue("--toast-scale")).toBe("1.4");
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.ui-style") ?? "{}").data.toastScale).toBe(1.4);
    // Stored values outside the bounds or of the wrong type are clamped or dropped on load.
    expect([mergeUiStyle({ toastScale: 9 }, DEFAULT_UI_STYLE).toastScale, mergeUiStyle({ toastScale: 0.1 }, DEFAULT_UI_STYLE).toastScale, mergeUiStyle({ toastScale: "big" }, DEFAULT_UI_STYLE).toastScale]).toEqual([TOAST_SCALE.max, TOAST_SCALE.min, DEFAULT_UI_STYLE.toastScale]);
  });
});
