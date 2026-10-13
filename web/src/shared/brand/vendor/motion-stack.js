// Three additional entrances for Stack, next to its original assembly and the
// shared blueprint. Same data model as `motion` in candidates-first.js:
// windows are fractions of the full-lockup duration, every part ends at its
// static state and has exactly one animation (fill: backwards would let a
// second one override the first before its delay).
//
// layers: vertical growth, the stack extrudes upwards, the bowl closes from
//         both ends, letters grow from the baseline
// dock:   horizontal docking from the right with a crisp overshoot, middle
//         module last, the bowl latches onto the stem
// relay:  no movement; modules wipe on bottom-up in the "next" colour and cool
//         to the primary, the bowl trace closes the circuit. The mark settles
//         before the letters start, "Docklet" settles before "Next" starts,
//         and nothing recolours the mark afterwards.

import { IN_OUT, OUT } from './shapes.js';

const MARK = 'var(--dln-c-mark)';
const NEXT = 'var(--dln-c-next)';
const WIPE_FROM = 'inset(100% 0% 0% 0%)';
const WIPE_TO = 'inset(0% 0% 0% 0%)';

/** Keyframes authored on the full timeline; `at` values become offsets of the window. */
function span(from, to, points) {
  return { from, to, frames: points.map(({ at, ...frame }) => ({ offset: Math.round(((at - from) / (to - from)) * 10000) / 10000, ...frame })) };
}

const extrude = (dy) => [
  { offset: 0, opacity: 0, transform: `translateY(${dy}px)`, easing: OUT },
  { offset: 0.12, opacity: 1 },
  { offset: 0.72, transform: 'translateY(-0.7px)', easing: IN_OUT },
  { offset: 1, opacity: 1, transform: 'none' },
];

const snapIn = (dx, overshoot) => [
  { offset: 0, opacity: 0, transform: `translateX(${dx}px)`, easing: OUT },
  { offset: 0.2, opacity: 1 },
  { offset: 0.6, transform: `translateX(${-overshoot}px)`, easing: IN_OUT },
  { offset: 0.82, transform: `translateX(${overshoot * 0.28}px)`, easing: IN_OUT },
  { offset: 1, opacity: 1, transform: 'none' },
];

/** Module that wipes on bottom-up in the "next" colour, then cools to the primary. */
function relayModule(part, from, label) {
  const lit = from + 0.12;
  const cooled = lit + 0.12;
  return {
    part,
    label,
    ...span(from, cooled, [
      { at: from, opacity: 0, clipPath: WIPE_FROM, fill: NEXT, easing: OUT },
      { at: from + 0.01, opacity: 1 },
      { at: lit, clipPath: WIPE_TO, fill: NEXT, easing: IN_OUT },
      { at: cooled, opacity: 1, clipPath: WIPE_TO, fill: MARK },
    ]),
  };
}

/** Relay phases as fractions: the mark settles, then "Docklet", then "Next". */
export const RELAY_PHASES = Object.freeze({ markEnd: 0.5, mainFrom: 0.5, mainTo: 0.78, nextFrom: 0.82 });

export const STACK_MOTIONS = {
  layers: {
    duration: 1500,
    markEnd: 0.8,
    parts: [
      {
        part: 'b1', from: 0, to: 0.22, origin: '50% 100%', label: 'Unteres Modul wächst aus der Grundlinie',
        frames: [
          { offset: 0, opacity: 0, transform: 'scaleY(0)', easing: OUT },
          { offset: 0.3, opacity: 1 },
          { offset: 0.68, transform: 'scaleY(1.05)', easing: IN_OUT },
          { offset: 1, opacity: 1, transform: 'none' },
        ],
      },
      { part: 'b2', from: 0.16, to: 0.42, label: 'Mittleres Modul schiebt sich aus dem unteren nach oben', frames: extrude(9) },
      { part: 'b3', from: 0.32, to: 0.58, label: 'Oberes Modul schiebt sich aus dem mittleren nach oben', frames: extrude(9) },
      {
        part: 'bowl', from: 0.5, to: 0.8, label: 'Bogen schließt sich von beiden Enden zur Mitte',
        frames: [
          { offset: 0, opacity: 0, strokeDasharray: '0 1 0 0', easing: IN_OUT },
          { offset: 0.06, opacity: 1 },
          { offset: 1, opacity: 1, strokeDasharray: '0.5 0 0.5 0' },
        ],
      },
    ],
    letters: { kind: 'grow', from: 0.56, to: 1, span: 0.36, label: 'Buchstaben wachsen gestaffelt aus der Grundlinie' },
  },
  dock: {
    duration: 1100,
    markEnd: 0.74,
    parts: [
      { part: 'b1', from: 0, to: 0.32, label: 'Unteres Modul fährt von rechts ein und rastet ein', frames: snapIn(9, 0.9) },
      { part: 'b3', from: 0.1, to: 0.42, label: 'Oberes Modul folgt', frames: snapIn(9, 0.9) },
      { part: 'b2', from: 0.2, to: 0.52, label: 'Mittleres Modul rastet zuletzt in die Lücke', frames: snapIn(9, 1.1) },
      { part: 'bowl', from: 0.44, to: 0.74, label: 'Bogen dockt am Stamm an', frames: snapIn(3.5, 0.7) },
    ],
    letters: { kind: 'snap', from: 0.5, to: 1, span: 0.26, label: 'Buchstaben rasten knapp nacheinander ein' },
  },
  relay: {
    duration: 1900,
    markEnd: RELAY_PHASES.markEnd,
    parts: [
      relayModule('b1', 0, 'Unteres Modul leuchtet in der Next-Farbe auf und kühlt ab'),
      relayModule('b2', 0.07, 'Mittleres Modul übernimmt das Signal'),
      relayModule('b3', 0.14, 'Oberes Modul übernimmt das Signal'),
      {
        part: 'bowl',
        label: 'Bogenspur schließt den Kreis und kühlt ab',
        ...span(0.22, RELAY_PHASES.markEnd, [
          { at: 0.22, opacity: 0, strokeDasharray: '1 1', strokeDashoffset: 1, stroke: NEXT, easing: 'linear' },
          { at: 0.23, opacity: 1 },
          { at: 0.42, strokeDashoffset: 0, stroke: NEXT, easing: IN_OUT },
          { at: RELAY_PHASES.markEnd, opacity: 1, strokeDasharray: '1 1', strokeDashoffset: 0, stroke: MARK },
        ]),
      },
    ],
    letters: {
      kind: 'relay',
      from: RELAY_PHASES.mainFrom,
      to: 1,
      span: 0.4,
      // "Docklet" lights up and settles before "Next" starts.
      runs: { main: [RELAY_PHASES.mainFrom, RELAY_PHASES.mainTo], next: [RELAY_PHASES.nextFrom, 1] },
      label: 'Wortmarke leuchtet von links nach rechts auf, „Next“ erst nach „Docklet“',
    },
  },
};

/** Letter animations of the Stack entrances; `relay` ends in each run's own colour. */
export const STACK_LETTER_KINDS = {
  grow: {
    selector: '',
    origin: '50% 100%',
    frames: [
      { offset: 0, opacity: 0, transform: 'scaleY(0.2)', easing: OUT },
      { offset: 0.6, opacity: 1, transform: 'scaleY(1.06)', easing: IN_OUT },
      { offset: 1, opacity: 1, transform: 'none' },
    ],
  },
  snap: {
    selector: '',
    frames: [
      { offset: 0, opacity: 0, transform: 'translateX(3px)', easing: OUT },
      { offset: 0.55, opacity: 1, transform: 'translateX(-0.35px)', easing: IN_OUT },
      { offset: 1, opacity: 1, transform: 'none' },
    ],
  },
  relay: {
    selector: ' path',
    frames: (run) => {
      const end = run === 'next' ? NEXT : 'var(--dln-c-main)';
      return [
        { offset: 0, opacity: 0, stroke: NEXT, easing: OUT },
        { offset: 0.18, opacity: 1, stroke: NEXT, easing: IN_OUT },
        { offset: 1, opacity: 1, stroke: end },
      ];
    },
  },
};
