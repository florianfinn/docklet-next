// Synthetic parser inputs, allowed only at this exact source path and category.
export const FIXTURES = {
  "non-example-email": ["operator@synthetic-mail.org"],
  "network-address": [
    "192.168.77.31",
    "10.99.0.1",
    "172.20.0.5",
    "100.64.0.1",
    "fd00::1234",
    "fe80::1234",
    "8.8.8.8",
  ],
  "private-hostname": ["fixture.lan", "fixture.home.arpa", "fixture.internal", "fixture.local"],
  "workstation-path": ["/home/synthetic/project", "/Users/synthetic/project"],
};
export function fixtureAllowed(path, category, value) {
  return path === "scripts/publication-fixtures.mjs" && FIXTURES[category]?.includes(value);
}
