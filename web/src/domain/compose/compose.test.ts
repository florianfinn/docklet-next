import { describe, expect, it } from "vitest";

import { convertDockerRun, DockerRunError } from "./docker-run";
import { parseCompose } from "./model";
import { ComposeEditError, mutate, ops } from "./ops";
import { validateCompose } from "./validate";

const SOURCE = `# Kopfkommentar bleibt
name: media
x-docklet:
  owner: familie
services:
  web:
    image: nginx:1.29-alpine # Inline-Kommentar
    ports:
      - "8080:80"
    environment:
      TZ: Europe/Berlin
    deploy:
      resources:
        limits:
          memory: 512m
    labels:
      - traefik.enable=true
volumes:
  data: {}
`;

describe("compose model", () => {
  it("reads known fields and lists unknown keys as extras", () => {
    const r = parseCompose(SOURCE);
    expect(r.ok).toBe(true);
    if (!r.ok) return;
    expect(r.model.name).toBe("media");
    expect(r.model.extras).toEqual(["x-docklet", "volumes"]);
    const web = r.model.services[0];
    expect(web?.image).toBe("nginx:1.29-alpine");
    expect(web?.ports[0]).toMatchObject({ host: "8080", container: "80", editable: true });
    expect(web?.extras).toEqual(["deploy", "labels"]);
  });

  it("reports parse errors with line numbers instead of a model", () => {
    const r = parseCompose("name: x\nservices:\n  web:\n    image: [unclosed\n");
    expect(r.ok).toBe(false);
    if (r.ok) return;
    expect(r.errors[0]?.line).toBeGreaterThan(0);
  });
});

describe("lossless form edits", () => {
  it("keeps comments, unknown keys and order when editing a field", () => {
    const next = mutate(SOURCE, ops.setField("web", "image", "nginx:1.30-alpine"));
    expect(next).toContain("# Kopfkommentar bleibt");
    expect(next).toContain("# Inline-Kommentar");
    expect(next).toContain("x-docklet:\n  owner: familie");
    expect(next).toContain("memory: 512m");
    expect(next).toContain("- traefik.enable=true");
    expect(next).toContain("image: nginx:1.30-alpine");
    expect(next.indexOf("x-docklet")).toBeLessThan(next.indexOf("services"));
  });

  it("round-trips Easy edits back to identical text when reverted", () => {
    const edited = mutate(SOURCE, ops.setField("web", "image", "nginx:9"));
    const reverted = mutate(edited, ops.setField("web", "image", "nginx:1.29-alpine"));
    expect(reverted).toBe(SOURCE);
  });

  it("adds and removes list entries without touching neighbours", () => {
    const withPort = mutate(SOURCE, ops.addPort("web"));
    const set = mutate(withPort, ops.setPort("web", 1, "8443", "443"));
    const r = parseCompose(set);
    expect(r.ok && r.model.services[0]?.ports.map((p) => p.raw)).toEqual(["8080:80", "8443:443"]);
    const removed = mutate(set, ops.removePort("web", 0));
    expect(removed).toContain('"8443:443"');
    expect(removed).not.toContain("8080:80");
  });

  it("refuses edits on invalid YAML so the raw draft is never overwritten", () => {
    const broken = "services:\n  web:\n    image: [oops\n";
    expect(() => mutate(broken, ops.setName("x"))).toThrow(ComposeEditError);
  });

  it("rejects renaming onto an existing service", () => {
    const two = mutate(SOURCE, ops.addService("db", { image: "postgres:17" }));
    expect(() => mutate(two, ops.renameService("db", "web"))).toThrow(ComposeEditError);
  });
});

describe("validation", () => {
  it("flags missing image and empty env values with codes", () => {
    const text = mutate(SOURCE, ops.addService("api", { image: "", environment: { TOKEN: "" } }));
    const r = parseCompose(text);
    if (!r.ok) throw new Error("expected valid YAML");
    const codes = validateCompose(r.model).map((i) => `${i.service}:${i.code}`);
    expect(codes).toContain("api:image-missing");
    expect(codes).toContain("api:env-value-empty");
  });
});

describe("docker run conversion", () => {
  it("maps flags to a service and reports what it could not take over", () => {
    const c = convertDockerRun('docker run -d --name web -p 8080:80 -v data:/data -e TZ=Europe/Berlin -e TOKEN --rm --cap-add NET_ADMIN nginx:1.29 nginx -g "daemon off;"', []);
    expect(c.key).toBe("web");
    expect(c.body).toMatchObject({ image: "nginx:1.29", container_name: "web", ports: ["8080:80"], volumes: ["data:/data"], environment: { TZ: "Europe/Berlin", TOKEN: "" } });
    expect(c.body.command).toEqual(["nginx", "-g", "daemon off;"]);
    const codes = c.hints.map((h) => h.code);
    expect(codes).toEqual(expect.arrayContaining(["env-no-value", "rm-ignored", "flag-ignored", "no-restart"]));
  });

  it("keeps the command as an argument list with POSIX quoting", () => {
    const cmd = (rest: string) => convertDockerRun(`docker run img ${rest}`, []).body.command;
    expect(cmd('printf "%s" "two words" ""')).toEqual(["printf", "%s", "two words", ""]);
    expect(cmd('nginx -g "daemon off;"')).toEqual(["nginx", "-g", "daemon off;"]);
    expect(cmd("a\\ b")).toEqual(["a b"]);
    expect(cmd('"a\\"b" "c\\d" "\\$x"')).toEqual(['a"b', "c\\d", "$x"]);
    expect(cmd("'a\\b $x'")).toEqual(["a\\b $x"]);
    expect(cmd("sh -c 'a && b'")).toEqual(["sh", "-c", "a && b"]);
    expect(cmd("echo $HOME")).toEqual(["echo", "$HOME"]);
  });

  it("handles backslash-newline continuation by quote state", () => {
    const cmd = (rest: string) => convertDockerRun(`docker run img ${rest}`, []).body.command;
    expect(cmd("foo\\\nbar")).toEqual(["foobar"]);
    expect(cmd("foo\\\r\nbar")).toEqual(["foobar"]);
    expect(cmd('"foo\\\nbar"')).toEqual(["foobar"]);
    expect(cmd("'foo\\\nbar'")).toEqual(["foo\\\nbar"]);
    expect(cmd("echo foo\\\n")).toEqual(["echo", "foo"]);
    const multi = convertDockerRun("docker run \\\n  -d \\\n  --name web \\\n  -p 8080:80 \\\n  nginx:1.29 \\\n  nginx", []);
    expect(multi.body).toMatchObject({ container_name: "web", ports: ["8080:80"], image: "nginx:1.29", command: ["nginx"] });
  });

  it("stops at unsupported shell syntax and reports the rest", () => {
    const run = (rest: string) => {
      const c = convertDockerRun(`docker run img ${rest}`, []);
      return [c.body.command, c.hints.filter((h) => h.code === "flag-ignored").map((h) => h.value)];
    };
    expect(run("cmd | grep x")).toEqual([["cmd"], ["| grep x"]]);
    expect(run("a && b")).toEqual([["a"], ["&& b"]]);
    expect(run("a; b")).toEqual([["a"], ["; b"]]);
    expect(run("a > out.txt")).toEqual([["a"], ["> out.txt"]]);
    expect(run("a $(b)")).toEqual([["a"], ["$(b)"]]);
    expect(run("a `b`")).toEqual([["a"], ["`b`"]]);
    expect(run('a "open b')).toEqual([["a"], ['"open b']]);
  });

  it("throws for commands that are not docker run", () => {
    expect(() => convertDockerRun("docker ps", [])).toThrow(DockerRunError);
  });
});
