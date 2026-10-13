// Centreline glyphs for the Docklet Next wordmarks. Original geometric
// letterforms drawn for this study; no font files or font outlines are used.
//
// Grid: baseline y = 20, x-height y = 10, ascender / cap height y = 5.
// Every path uses absolute commands (M L H V A Q Z) so it can be offset
// numerically. Each glyph lists its strokes separately so draw-on animations
// can give every stroke its own normalised length.

export const METRICS = Object.freeze({ capTop: 5, xTop: 10, baseline: 20 });

const LOWER_ROUND = {
  d: { w: 10, strokes: ['M10 15 A5 5 0 1 1 0 15 A5 5 0 1 1 10 15', 'M10 5 V20'] },
  o: { w: 10, strokes: ['M5 10 A5 5 0 1 0 5 20 A5 5 0 1 0 5 10'] },
  c: { w: 9, strokes: ['M8.54 11.46 A5 5 0 1 0 8.54 18.54'] },
  k: { w: 7.3, strokes: ['M0 5 V20', 'M7 10 L0 16', 'M2.8 13.6 L7.3 20'] },
  l: { w: 0, strokes: ['M0 5 V20'] },
  e: { w: 10, strokes: ['M0 15 H10 A5 5 0 1 0 8.54 18.54'] },
  t: { w: 5.5, strokes: ['M2.5 6 V17 Q2.5 20 5.5 20', 'M0 10 H5.5'] },
  n: { w: 9, strokes: ['M0 10 V20', 'M0 14.5 A4.5 4.5 0 0 1 9 14.5 V20'] },
  x: { w: 8, strokes: ['M0 10 L8 20', 'M8 10 L0 20'] },
};

const UPPER_SQUARE = {
  D: { w: 12, strokes: ['M0 5 H7.5 A4.5 4.5 0 0 1 12 9.5 V15.5 A4.5 4.5 0 0 1 7.5 20 H0 Z'] },
  O: {
    w: 12,
    strokes: ['M4.5 5 H7.5 A4.5 4.5 0 0 1 12 9.5 V15.5 A4.5 4.5 0 0 1 7.5 20 H4.5 A4.5 4.5 0 0 1 0 15.5 V9.5 A4.5 4.5 0 0 1 4.5 5 Z'],
  },
  C: { w: 11, strokes: ['M11 5 H4.5 A4.5 4.5 0 0 0 0 9.5 V15.5 A4.5 4.5 0 0 0 4.5 20 H11'] },
  K: { w: 10.5, strokes: ['M0 5 V20', 'M0 12.5 H3.5', 'M10.5 5 L3.5 12.5 L10.5 20'] },
  L: { w: 9, strokes: ['M0 5 V20 H9'] },
  E: { w: 10, strokes: ['M10 5 H0 V20 H10', 'M0 12.5 H8'] },
  T: { w: 11, strokes: ['M0 5 H11', 'M5.5 5 V20'] },
  N: { w: 11, strokes: ['M0 20 V5 L11 20 V5'] },
  X: { w: 11, strokes: ['M0 5 L11 20', 'M11 5 L0 20'] },
};

export const GLYPH_SETS = Object.freeze({ lowerRound: LOWER_ROUND, upperSquare: UPPER_SQUARE });

/** Formats a coordinate with at most three decimals and no trailing zeros. */
export function num(value) {
  return String(Math.round(value * 1000) / 1000);
}

/** Offsets and scales an absolute-command path; arc radii scale, flags stay. */
export function transformPath(d, dx = 0, dy = 0, s = 1) {
  const tokens = d.match(/[MLHVAQZ]|-?\d*\.?\d+(?:e-?\d+)?/gi) ?? [];
  const out = [];
  let cmd = '';
  let args = [];
  const flush = () => {
    if (!cmd) return;
    if (cmd === 'Z') {
      out.push('Z');
      return;
    }
    const n = args.map(Number);
    const X = (v) => num(v * s + dx);
    const Y = (v) => num(v * s + dy);
    const R = (v) => num(v * s);
    const step = { M: 2, L: 2, Q: 4, H: 1, V: 1, A: 7 }[cmd];
    for (let i = 0; i < n.length; i += step) {
      const a = n.slice(i, i + step);
      if (cmd === 'H') out.push(`H${X(a[0])}`);
      else if (cmd === 'V') out.push(`V${Y(a[0])}`);
      else if (cmd === 'A') out.push(`A${R(a[0])} ${R(a[1])} ${a[2]} ${a[3]} ${a[4]} ${X(a[5])} ${Y(a[6])}`);
      else if (cmd === 'Q') out.push(`Q${X(a[0])} ${Y(a[1])} ${X(a[2])} ${Y(a[3])}`);
      else out.push(`${cmd}${X(a[0])} ${Y(a[1])}`);
    }
  };
  for (const token of tokens) {
    if (/^[A-Z]$/i.test(token)) {
      flush();
      cmd = token.toUpperCase();
      args = [];
    } else {
      args.push(token);
    }
  }
  flush();
  return out.join(' ');
}

/**
 * Lays out a word on the glyph grid. `tracking` is the gap between advance
 * boxes on the centreline; the visible gap is tracking minus stroke width.
 */
export function layoutWord(text, setName, { tracking = 4, space = 6, x = 0, y = 0 } = {}) {
  const set = GLYPH_SETS[setName];
  const letters = [];
  let cursor = x;
  for (const char of text) {
    if (char === ' ') {
      cursor += space - tracking;
      continue;
    }
    const glyph = set[char];
    if (!glyph) throw new Error(`Glyph "${char}" is missing in ${setName}`);
    letters.push({ char, x: cursor, strokes: glyph.strokes.map((d) => transformPath(d, cursor, y)) });
    cursor += glyph.w + tracking;
  }
  return { letters, width: Math.max(0, cursor - tracking - x) };
}
