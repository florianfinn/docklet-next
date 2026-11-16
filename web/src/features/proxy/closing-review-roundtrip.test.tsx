// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing review (C-B1, C-B4, C-N2) counterexamples.

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { InstanceWorkspace } from "./InstanceWorkspace";
import { patchCaddySite } from "./caddy-draft";
import { fixture, instance, Providers, route } from "./proxy.test-support";

const base =
  "# docklet-route: app\nhttp://app.example.com {\n  route {\n    reverse_proxy http://backend.example.com:8080\n  }\n}\n";
const variants = {
  CRLF: base.replaceAll("\n", "\r\n"),
  trailing: base.replaceAll("\n", "  \n"),
  "CRLF-matched":
    "# docklet-route: app\r\n# docklet-certificate-domain: example.com\r\napp.example.com, example.com, *.example.com {\r\n  tls {\r\n    dns cloudflare {env.EXAMPLE_DNS_TOKEN}\r\n  }\r\n  @docklet {\r\n    host app.example.com\r\n  }\r\n  handle @docklet {\r\n    route {\r\n      reverse_proxy http://backend.example.com:8080\r\n    }\r\n  }\r\n}\r\n",
  duplicate: base.replace(
    "http://backend.example.com:8080\n",
    "http://backend.example.com:8080 http://backend.example.com:8080 {\n      lb_policy round_robin\n    }\n",
  ),
};
for (const [format, text] of Object.entries(variants)) {
  for (const leaf of ["domain", "upstream"] as const) {
    it(`reviewer applies Caddy ${format} ${leaf} through the editor`, async () => {
      const f = fixture();
      const r = {
        ...route,
        ...(format === "duplicate" ? { upstreams: [route.upstreams![0]!, route.upstreams![0]!] } : {}),
        id: "routes/app.caddy",
        file: "routes/app.caddy",
        rule: "app.example.com",
      };
      f.state.read.routes = [r];
      f.state.read.files = [{ ...f.state.read.files![0]!, path: r.file, content: text }];
      render(
        <Providers>
          <InstanceWorkspace
            instance={{ ...instance, kind: "caddy" }}
            api={f.api}
            updated={() => {}}
            removed={() => {}}
          />
        </Providers>,
      );
      const user = userEvent.setup();
      await user.click(await screen.findByRole("button", { name: "Actions for app" }));
      await user.click(await screen.findByRole("menuitem", { name: "Edit route" }));
      fireEvent.change(screen.getByLabelText(leaf === "domain" ? "Host / domain" : "Backend host"), {
        target: { value: "next.example.com" },
      });
      await user.click(screen.getByRole("button", { name: "Save" }));
      await waitFor(() => expect(f.writes.filter(w => w.path.endsWith("/operations")).length).toBe(1));
      const sent = f.writes.find(w => w.path.endsWith("/operations"))!.body as { change: { advanced: string } };
      expect(sent.change.advanced).toBe(
        leaf === "domain"
          ? text.replaceAll("app.example.com", "next.example.com")
          : text.replace("backend.example.com", "next.example.com"),
      );
    });
  }
}
it("reviewer replaces just the first identical Caddy upstream", () => {
  const text = base.replace(
    "http://backend.example.com:8080\n",
    "http://backend.example.com:8080 http://backend.example.com:8080 {\n      lb_policy round_robin\n    }\n",
  );
  const r = { ...route, rule: "app.example.com", upstreams: [route.upstreams![0]!, route.upstreams![0]!] };
  expect(patchCaddySite(text, r, "app", r.rule, "http://next.example.com:8080")).toBe(
    text.replace("http://backend.example.com:8080", "http://next.example.com:8080"),
  );
});

it("reviewer hides newline middleware definitions from the line editor", async () => {
  const f = fixture();
  f.state.read.middlewares = [{ name: "headers\nstrict", kind: "local", file: route.file }];
  render(
    <Providers>
      <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} />
    </Providers>,
  );
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Actions for app" }));
  await user.click(await screen.findByRole("menuitem", { name: "Edit route" }));
  screen.getByRole("combobox", { name: "Existing middleware" }).focus();
  await user.keyboard("{ArrowDown}");
  await screen.findByRole("option", { name: "headers" });
  expect(screen.getAllByRole("option").some(el => el.textContent?.includes("\n"))).toBe(false);
});

it("reviewer freezes the middleware picker with a frozen multiline chain", async () => {
  const f = fixture();
  f.state.read.routes![0] = { ...route, references: [{ name: "headers\nstrict", kind: "local", file: route.file }] };
  f.state.read.middlewares = [{ name: "extra", kind: "local", file: route.file }];
  render(
    <Providers>
      <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} />
    </Providers>,
  );
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Actions for app" }));
  await user.click(await screen.findByRole("menuitem", { name: "Edit route" }));
  const picker = screen.getByRole("combobox", { name: "Existing middleware" });
  expect(picker.hasAttribute("disabled")).toBe(true);
});
