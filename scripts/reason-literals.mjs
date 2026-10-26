// Source: docklet-next/docs/design/update-policies.md
// Revision: 8907719aba6cd2d3037d287729f689d5ad9e3ec9
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Inspect Go reason helper arguments and returns without prefix assumptions.

export function reasonLiterals(source) {
  const tokens = [...source.matchAll(/\/\/[^\n]*|\/\*[\s\S]*?\*\/|"(?:\\.|[^"\\])*"|`[^`]*`|[a-zA-Z_]\w*|[^\s]/g)]
    .map(([token]) => token).filter((token) => !token.startsWith("//") && !token.startsWith("/*"));
  const literal = (token) => token?.startsWith('"') ? JSON.parse(token) : token?.startsWith("`") ? token.slice(1, -1) : null;
  const codes = [];
  const argument = { policyTarget: 2, policyTargetWithActivity: 3, policyCode: 1 };
  for (let i = 0; i < tokens.length; i++) {
    if (tokens[i] === "func" && tokens[i + 1] === "policyCode") {
      let body = i + 2;
      while (body < tokens.length && tokens[body] !== "{") body++;
      let depth = 1;
      for (let j = body + 1; j < tokens.length && depth; j++) {
        if (tokens[j] === "{") depth++;
        if (tokens[j] === "}") depth--;
        if (tokens[j] === "return" && literal(tokens[j + 1]) !== null) codes.push(literal(tokens[j + 1]));
      }
    }
    if (!Object.hasOwn(argument, tokens[i]) || tokens[i + 1] !== "(" || tokens[i - 1] === "func") continue;
    let depth = 1, index = 0;
    for (let j = i + 2; j < tokens.length && depth; j++) {
      const token = tokens[j];
      if (depth === 1 && index === argument[tokens[i]] && literal(token) !== null) codes.push(literal(token));
      if (["(", "[", "{"].includes(token)) depth++;
      if ([")", "]", "}"].includes(token)) depth--;
      if (depth === 1 && token === ",") index++;
    }
  }
  return codes.filter((code) => code !== "");
}
