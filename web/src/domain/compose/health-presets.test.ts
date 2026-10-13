import { describe, expect, it } from "vitest";
import { parse } from "yaml";

import { formatArgs, healthPresets, imageName, parseArgs, presetCommand } from "./health-presets";
import { parseCompose } from "./model";
import { mutate, ops } from "./ops";
import { validateCompose } from "./validate";

const svc = (image: string, ports: string[] = []) => ({ image, ports: ports.map((container) => ({ container })) });
const model = (text: string) => {
  const r = parseCompose(text);
  if (!r.ok) throw new Error("parse");
  return r.model;
};

const WITH_HEALTH = `services:
  db:
    image: postgres:17-alpine
    healthcheck:
      # eigene Prüfung
      test: ["CMD", "pg_isready", "-U", "app"]
      interval: 10s
      x-note: bleibt
`;

describe("healthcheck presets", () => {
  it("match only official images whose tool ships in the image, else offer the two generic HTTP checks", () => {
    expect([imageName("docker.io/library/postgres:17@sha256:abc"), imageName("ghcr.io/x/redis-like:1")]).toEqual(["postgres", "redis-like"]);
    expect(["postgres:17", "docker.io/library/mariadb:11", "mysql:8.4", "redis:7.4-alpine", "valkey/valkey:8"].map((i) => healthPresets(svc(i)).map((p) => [p.id, p.form, p.matched]))).toEqual([
      [["postgres", "CMD-SHELL", true]],
      [["mariadb", "CMD", true]],
      [["mysql", "CMD", true]],
      [["redis", "CMD", true]],
      [["valkey", "CMD", true]],
    ]);
    // A different vendor's image is not assumed to carry the official tool.
    const generic = healthPresets(svc("bitnami/postgresql:17", ["5432"]));
    expect(generic.map((p) => [p.id, p.requires, presetCommand(p)])).toEqual([
      ["http-curl", ["curl"], "curl -fsS http://localhost:5432/ || exit 1"],
      ["http-wget", ["wget"], "wget -q --spider http://localhost:5432/ || exit 1"],
    ]);
    expect(presetCommand(healthPresets(svc("nginx:1.29"))[0]!)).toBe("curl -fsS http://localhost:80/ || exit 1");
    expect(presetCommand(healthPresets(svc("redis:7"))[0]!)).toBe('["redis-cli", "ping"]');
  });

  it("apply on top of an existing healthcheck without losing comments or unknown keys, in Compose's exec or shell form", () => {
    const [preset] = healthPresets(svc("redis:7"));
    const out = mutate(WITH_HEALTH, ops.applyHealthPreset("db", preset!));
    expect(out).toContain("# eigene Prüfung");
    expect(out).toContain("x-note: bleibt");
    expect(out).toContain("test: [ CMD, redis-cli, ping ]");
    const health = model(out).services[0]?.health;
    expect([health?.form, health?.args, health?.interval, health?.retries]).toEqual(["CMD", ["redis-cli", "ping"], "30s", "3"]);
    const pg = mutate(WITH_HEALTH, ops.applyHealthPreset("db", healthPresets(svc("postgres:17"))[0]!));
    // $$ is Compose's escape: the container shell reads the variable at check time.
    expect((parse(pg) as { services: { db: { healthcheck: { test: string[] } } } }).services.db.healthcheck.test).toEqual(["CMD-SHELL", 'pg_isready -U "$${POSTGRES_USER:-postgres}"']);
    expect(pg).toContain("start_period: 20s");
  });

  it("keep CMD arguments exact: spaces, empty strings, quotes and backslashes survive writing and reading", () => {
    const args = ["tool", "--label", "hello world", ""];
    const tricky = ["sh-free", 'say "hi"', "C:\\path\\x", "it's", "a\tb"];
    for (const a of [args, tricky]) {
      const out = mutate(WITH_HEALTH, ops.setHealthArgs("db", a));
      expect((parse(out) as { services: { db: { healthcheck: { test: string[] } } } }).services.db.healthcheck.test).toEqual(["CMD", ...a]);
      expect([model(out).services[0]?.health.args, parseArgs(formatArgs(a))]).toEqual([a, a]);
      expect([out.includes("# eigene Prüfung"), out.includes("x-note: bleibt")]).toEqual([true, true]);
    }
    // Editing timing leaves an existing exec array's values and quoting untouched (the YAML writer only pads the brackets).
    const original = WITH_HEALTH.replace('["CMD", "pg_isready", "-U", "app"]', '["CMD", "tool", "--label", "hello world", ""]');
    const timed = mutate(original, ops.setHealthField("db", "retries", "5"));
    expect([timed.includes('test: [ "CMD", "tool", "--label", "hello world", "" ]'), model(timed).services[0]?.health.args]).toEqual([true, ["tool", "--label", "hello world", ""]]);
    // Only a JSON list of strings counts; anything else is rejected rather than guessed.
    expect(['["a", ""]', "[]", "a b", '["a", 1]', '{"a":1}', '["a"'].map(parseArgs)).toEqual([["a", ""], [], undefined, undefined, undefined, undefined]);
    // An exec test without a program is missing; an empty trailing argument is fine.
    const missing = (test: string) => validateCompose(model(WITH_HEALTH.replace('["CMD", "pg_isready", "-U", "app"]', test))).some((i) => i.code === "health-test-missing");
    expect([missing('["CMD"]'), missing('["CMD", ""]'), missing('["CMD", "tool", ""]')]).toEqual([true, true, false]);
  });

  it("keeps CMD-SHELL and plain-string tests in their form when edited", () => {
    const shell = WITH_HEALTH.replace('["CMD", "pg_isready", "-U", "app"]', '["CMD-SHELL", "pg_isready"]');
    expect(mutate(shell, ops.setHealthTest("db", "pg_isready -U \"a b\"", "CMD-SHELL"))).toContain('test: [ CMD-SHELL, pg_isready -U "a b" ]');
    const str = WITH_HEALTH.replace('test: ["CMD", "pg_isready", "-U", "app"]', "test: pg_isready");
    expect(model(str).services[0]?.health.form).toBe("string");
    expect(mutate(str, ops.setHealthTest("db", "pg_isready -q", "string"))).toContain("test: pg_isready -q");
  });

  it("validate interval as a Compose duration and retries as a whole number", () => {
    const codes = (interval: string, retries: string) =>
      validateCompose(model(WITH_HEALTH.replace("interval: 10s", `interval: ${interval}\n      retries: ${retries}`)))
        .map((i) => i.code)
        .filter((c) => c.startsWith("health"));
    expect([codes("1m30s", "3"), codes("30", "3"), codes("30s", "0"), codes("500ms", "two")]).toEqual([[], ["health-interval-format"], ["health-retries-format"], ["health-retries-format"]]);
  });
});
