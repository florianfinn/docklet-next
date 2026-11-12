// Source: docklet-next/issues/235
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original SemVer 2 precedence for hub and agent versions; mirrors types/version.

type Product = { core: [bigint, bigint, bigint]; pre: string[] };

const NUMERIC = /^(0|[1-9]\d*)$/;
const IDENTIFIER = /^[0-9A-Za-z-]+$/;

function parse(raw: string): Product | undefined {
  const [main = "", build, extra] = raw.trim().replace(/^v/, "").split("+");
  if (extra !== undefined || (build !== undefined && !build.split(".").every((id) => IDENTIFIER.test(id)))) return undefined;
  const dash = main.indexOf("-");
  const core = (dash < 0 ? main : main.slice(0, dash)).split(".");
  if (core.length !== 3 || !core.every((part) => NUMERIC.test(part))) return undefined;
  const pre = dash < 0 ? [] : main.slice(dash + 1).split(".");
  if (dash >= 0 && !pre.every((id) => IDENTIFIER.test(id) && (!/^\d+$/.test(id) || NUMERIC.test(id)))) return undefined;
  return { core: [BigInt(core[0]!), BigInt(core[1]!), BigInt(core[2]!)], pre };
}

function compareIdentifier(a: string, b: string): number {
  const an = /^\d+$/.test(a), bn = /^\d+$/.test(b);
  if (an && bn) return BigInt(a) === BigInt(b) ? 0 : BigInt(a) < BigInt(b) ? -1 : 1;
  if (an) return -1;
  if (bn) return 1;
  return a === b ? 0 : a < b ? -1 : 1;
}

/** SemVer 2 precedence (1.0.0-rc.1 < 1.0.0-rc.2 < 1.0.0); undefined when either value is invalid. */
export function compareProductVersion(a: string, b: string): number | undefined {
  const x = parse(a), y = parse(b);
  if (!x || !y) return undefined;
  for (let i = 0; i < 3; i++) if (x.core[i] !== y.core[i]) return x.core[i]! < y.core[i]! ? -1 : 1;
  if (!x.pre.length || !y.pre.length) return x.pre.length === y.pre.length ? 0 : x.pre.length ? -1 : 1;
  for (let i = 0; i < Math.min(x.pre.length, y.pre.length); i++) {
    const c = compareIdentifier(x.pre[i]!, y.pre[i]!);
    if (c) return c;
  }
  return x.pre.length === y.pre.length ? 0 : x.pre.length < y.pre.length ? -1 : 1;
}
