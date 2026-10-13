// Round-one candidates: Berth, Stack and Forward.
// Marks use a 32-unit grid; `small` is a separate optical master on a
// 16-unit pixel grid for 16–20 px rendering; `ink` is the painted bounding
// box [x0, y0, x1, y1] used to crop lockups; `guides` are construction lines
// for the blueprint entrance. Motion windows are fractions of the full-lockup
// duration; keyframe easing applies to the segment that starts at that
// keyframe (the same rule in WAAPI and in CSS @keyframes).

import { STACK_MOTIONS } from './motion-stack.js';
import { IN_OUT, OUT, band, dropIn, path, rect, slideIn } from './shapes.js';

export const FIRST_ROUND = [
  {
    id: 'berth',
    name: 'Berth',
    summary: 'Ein Container senkt sich in seinen Liegeplatz. Becken und Kai bilden zusammen ein kleines d.',
    idea: 'Ein oben offenes Hafenbecken mit hohem Kai bildet die Form des d. Ein Containermodul liegt im Becken. Der Rundumabstand zeigt das Anlegen, und die Silhouette bleibt als d lesbar.',
    strengths: [
      'Liest sich als d, also als Docklet, und zeigt dazu ein klares Hafenbild: ein Container im Liegeplatz.',
      'Geschlossene, kompakte Silhouette. Auch der 16-px-Master bleibt eindeutig.',
      'Ruhige Wortmarke in Kleinbuchstaben. „next“ ist leichter gesetzt, die Hierarchie trägt also auch einfarbig.',
    ],
    risks: [
      'Die einzeilige Wortmarke ist breit. In schmalen Sidebars braucht es die Kompaktvariante.',
      'Ein Quadrat im Bauch eines d kommt bei Buchstabenlogos öfter vor. Die offene Oberkante des Beckens muss bleiben.',
    ],
    mark: {
      regular: [
        path('body', 'M22 6 A3 3 0 0 1 28 6 V26 A3 3 0 0 1 25 29 H10.5 A6.5 6.5 0 0 1 4 22.5 V15.25 A2.25 2.25 0 0 1 8.5 15.25 V22.5 A2 2 0 0 0 10.5 24.5 H22 Z', 'basin'),
        rect('box', 10.5, 13, 9.5, 9.5, 1.5),
      ],
      small: [
        path('body', 'M11 2.5 A1.5 1.5 0 0 1 14 2.5 V13.5 A1.5 1.5 0 0 1 12.5 15 H4 A3 3 0 0 1 1 12 V7 A1 1 0 0 1 3 7 V12 A1 1 0 0 0 4 13 H11 Z', 'basin'),
        rect('box', 4, 6, 6, 6, 1),
      ],
      ink: { regular: [4, 3, 28, 29], small: [1, 1, 14, 15] },
      guides: [{ h: 29 }, { h: 3 }, { v: 4 }, { v: 22 }, { h: 13 }, { circle: [10.5, 22.5, 6.5] }],
    },
    wordmark: {
      set: 'lowerRound', main: 'docklet', next: 'next', placement: 'inline',
      weight: 2.2, nextWeight: 1.5, tracking: 4.6, space: 10, scale: 1.18,
      linecap: 'round', linejoin: 'round', gap: 8, align: { baseline: 29 },
      compact: { scale: 1, nextScale: 0.62, nextWeight: 2.6, nextTracking: 8, lineGap: 5.4, center: 16 },
    },
    motion: {
      duration: 1200,
      markEnd: 0.7,
      parts: [
        {
          part: 'body', from: 0, to: 0.34, origin: '50% 100%', label: 'Becken und Kai wachsen aus der Grundlinie',
          frames: [
            { offset: 0, opacity: 0, transform: 'scaleY(0)', easing: OUT },
            { offset: 0.2, opacity: 1 },
            { offset: 0.75, transform: 'scaleY(1.04)', easing: IN_OUT },
            { offset: 1, opacity: 1, transform: 'none' },
          ],
        },
        {
          part: 'box', from: 0.22, to: 0.64, label: 'Container senkt sich wie am Kran in den Liegeplatz',
          frames: [
            { offset: 0, opacity: 0, transform: 'translateY(-15px)', easing: OUT },
            { offset: 0.3, opacity: 1 },
            { offset: 0.74, transform: 'translateY(0.8px)', easing: IN_OUT },
            { offset: 1, opacity: 1, transform: 'none' },
          ],
        },
        {
          part: 'basin', from: 0.52, to: 0.7, origin: '50% 100%', label: 'Becken federt beim Aufsetzen kurz nach',
          frames: [
            { offset: 0, transform: 'none', easing: OUT },
            { offset: 0.35, transform: 'scaleY(0.97)', easing: IN_OUT },
            { offset: 1, transform: 'none' },
          ],
        },
      ],
      letters: { kind: 'draw', from: 0.42, to: 1, span: 0.45, label: 'Buchstaben zeichnen sich nacheinander' },
    },
  },
  {
    id: 'stack',
    name: 'Stack',
    summary: 'Drei gestapelte Module bilden den Stamm, ein Bogen schließt das D.',
    idea: 'Ein großes D aus drei gestapelten Modulen und einem Bogen. Die Module stehen für Stacks und Modularität. Die Wortmarke ist zweizeilig und dadurch kompakt.',
    strengths: [
      'Großes D aus drei Modulen, sofort als D lesbar. Der Stapel steht direkt für Compose-Stacks.',
      'Die zweizeilige Wortmarke ist von Haus aus kompakt und passt in eine 160 × 36-Fläche.',
      'Fallen, Stapeln und Bogen erzählen „setzt sich zusammen“ am deutlichsten.',
    ],
    risks: [
      'Bei 16 px wirken die drei Module leicht wie eine Punktreihe vor einem D.',
      'Die Wortmarke in Versalien wirkt technischer und kühler. „NEXT“ wird in kleinen Größen schnell dünn.',
    ],
    mark: {
      regular: [
        rect('b3', 4, 3, 7, 7, 1.5),
        rect('b2', 4, 12, 7, 7, 1.5),
        rect('b1', 4, 21, 7, 7, 1.5),
        band('bowl', 'M13 5.5 H16 A10 10 0 0 1 16 25.5 H13', 5),
      ],
      small: [
        rect('b3', 1, 1, 4, 4, 1),
        rect('b2', 1, 6, 4, 4, 1),
        rect('b1', 1, 11, 4, 4, 1),
        band('bowl', 'M6 2.5 H8 A5.5 5.5 0 0 1 8 13.5 H6', 3),
      ],
      ink: { regular: [4, 3, 28.5, 28], small: [1, 1, 15, 15] },
      guides: [{ v: 4 }, { v: 11 }, { h: 3 }, { h: 28 }, { h: 15.5 }, { circle: [16, 15.5, 10] }],
    },
    wordmark: {
      set: 'upperSquare', main: 'DOCKLET', next: 'NEXT', placement: 'below',
      weight: 2.5, nextWeight: 2.6, tracking: 4.4, nextTracking: 7, scale: 0.92, nextScale: 0.5, lineGap: 3.6,
      linecap: 'square', linejoin: 'miter', gap: 8, align: { center: 15.75 },
    },
    // The mark is a capital D, so two lockups let it stand in for the D of
    // DOCKLET: `initial` at cap height, `dropcap` spanning both lines.
    // Gaps are visible distances in lockup units.
    initial: { letter: 'D', main: 'OCKLET', gap: { initial: 2, dropcap: 3 } },
    motions: STACK_MOTIONS,
    motion: {
      duration: 1400,
      markEnd: 0.78,
      parts: [
        { part: 'b1', from: 0, to: 0.26, origin: '50% 100%', label: 'Unteres Modul fällt ein und federt', frames: dropIn(-14) },
        { part: 'b2', from: 0.12, to: 0.38, origin: '50% 100%', label: 'Mittleres Modul stapelt sich darauf', frames: dropIn(-14) },
        { part: 'b3', from: 0.24, to: 0.5, origin: '50% 100%', label: 'Oberes Modul schließt den Stapel', frames: dropIn(-14) },
        {
          part: 'bowl', from: 0.42, to: 0.78, label: 'Bogen zeichnet sich von oben nach unten',
          frames: [
            { offset: 0, opacity: 0, strokeDasharray: '1 1', strokeDashoffset: 1, easing: IN_OUT },
            { offset: 0.06, opacity: 1 },
            { offset: 1, opacity: 1, strokeDasharray: '1 1', strokeDashoffset: 0 },
          ],
        },
      ],
      letters: { kind: 'rise', from: 0.48, to: 1, span: 0.4, label: 'Buchstaben steigen gestaffelt auf' },
    },
  },
  {
    id: 'forward',
    name: 'Forward',
    summary: 'Container-Riegel versetzen sich zu einem Vorwärtspfeil. Das letzte Modul dockt als Spitze an.',
    idea: 'Drei Container-Riegel. Der mittlere schiebt sich nach vorn, und ein kurzes Modul dockt mit Abstand als Spitze an. Das steht für „das nächste Modul“, also „Next“. Die Wortmarke ist kräftig und kursiv. Das Zeichen ist bewusst kein Buchstabe.',
    strengths: [
      'Stärkste „Next“-Aussage und sehr dynamisch. Die kursive Wortmarke wirkt schnell.',
      'Besteht nur aus Rechtecken und ist damit bei 16 px und in Monochrom am robustesten.',
    ],
    risks: [
      'Kein Buchstabe: Der Bezug zu Docklet entsteht nur über die Wortmarke.',
      'Kann in einer Navigation wie ein Ausrichtungs- oder Listen-Icon gelesen werden.',
    ],
    mark: {
      regular: [rect('bar1', 4, 4.5, 16, 6, 2), rect('bar2', 12, 13, 10.5, 6, 2), rect('nose', 24, 13, 4, 6, 1.5), rect('bar3', 4, 21.5, 16, 6, 2)],
      small: [rect('bar1', 1, 1, 9, 4, 1), rect('bar2', 6, 6, 6, 4, 1), rect('nose', 13, 6, 2, 4, 0.75), rect('bar3', 1, 11, 9, 4, 1)],
      ink: { regular: [4, 4.5, 28, 27.5], small: [1, 1, 15, 15] },
      guides: [{ h: 7.5 }, { h: 16 }, { h: 24.5 }, { v: 4 }, { line: [4, 4.5, 28, 16] }, { line: [4, 27.5, 28, 16] }],
    },
    wordmark: {
      set: 'lowerRound', main: 'docklet', next: 'next', placement: 'inline',
      weight: 2.9, nextWeight: 2.9, tracking: 5.1, space: 12, scale: 1.14, skew: -10,
      linecap: 'round', linejoin: 'round', gap: 8, align: { baseline: 27.5 },
      compact: { scale: 0.98, nextScale: 0.62, nextWeight: 3.7, nextTracking: 8.5, lineGap: 5.6, center: 16 },
    },
    motion: {
      duration: 1000,
      markEnd: 0.66,
      parts: [
        { part: 'bar1', from: 0, to: 0.36, label: 'Oberer Riegel fährt von links ein', frames: slideIn(-18, 0.8) },
        { part: 'bar3', from: 0.07, to: 0.43, label: 'Unterer Riegel folgt', frames: slideIn(-18, 0.8) },
        { part: 'bar2', from: 0.15, to: 0.53, label: 'Mittlerer Riegel schiebt sich nach vorn', frames: slideIn(-22, 1.4) },
        { part: 'nose', from: 0.3, to: 0.66, label: 'Kurzes Modul dockt als Pfeilspitze an', frames: slideIn(-28, 2.2) },
      ],
      letters: { kind: 'slide', from: 0.36, to: 1, span: 0.42, label: 'Buchstaben gleiten kursiv nach' },
    },
  },
];
