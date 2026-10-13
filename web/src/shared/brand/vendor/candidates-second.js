// Round-two candidates: Slot, Orbit and Cradle. Same data model as
// candidates-first.js; the geometry is derived from a few named measures so
// the construction guides and the shapes share one source.

import { FALL, IN_OUT, OUT, band, circle, diamondPath, path, popIn, rect, settle } from './shapes.js';

const drawOn = (easing = IN_OUT) => [
  { offset: 0, opacity: 0, strokeDasharray: '1 1', strokeDashoffset: 1, easing },
  { offset: 0.06, opacity: 1 },
  { offset: 1, opacity: 1, strokeDasharray: '1 1', strokeDashoffset: 0 },
];

/** V-shaped cradle at 45° with a diamond whose lower edges keep an even gap to the arms. */
function cradleGeometry({ cx, vertexY, arm, width, side, gap, radius }) {
  const centerY = vertexY - Math.SQRT2 * (width / 2 + gap + side / 2);
  const round = (v) => Math.round(v * 1000) / 1000;
  return {
    dock: `M${round(cx - arm)} ${round(vertexY - arm)} L${cx} ${vertexY} L${round(cx + arm)} ${round(vertexY - arm)}`,
    gem: diamondPath(cx, centerY, side, radius),
    centerY: round(centerY),
  };
}

const CRADLE = cradleGeometry({ cx: 16, vertexY: 25.2, arm: 10.5, width: 4.2, side: 11, gap: 1.8, radius: 2.2 });
const CRADLE_SMALL = cradleGeometry({ cx: 8, vertexY: 12.6, arm: 5.2, width: 2.2, side: 5.6, gap: 1, radius: 1 });

export const SECOND_ROUND = [
  {
    id: 'slot',
    name: 'Slot',
    summary: 'Ein Modul rastet in die offene Ecke einer Kachel ein und schließt die Fläche.',
    idea: 'Eine volle Kachel mit ausgesparter Ecke. Das Modul sitzt mit gleichmäßiger L-Fuge im Steckplatz und vervollständigt die Fläche. Das Thema ist Erweiterbarkeit: Jedes Modul hat seinen Platz. Kein Buchstabe, aber die geschlossenste Silhouette der Studie.',
    strengths: [
      'Massivste Form: Bei 16 px, einfarbig und als App-Icon am kräftigsten.',
      'Liest sich sofort als „Modul rastet ein“. Die L-förmige Fuge ist das eigene Detail.',
      'Füllt die Fläche und taugt ohne Umbau als Favicon. Die Wortmarke kombiniert kräftige Kleinbuchstaben mit Kapitälchen.',
    ],
    risks: [
      'Kachel mit Eckmodul ist ein verbreitetes Motiv (Widget-, Badge- oder Bild-in-Bild-Symbole). Eine Ähnlichkeitsrecherche ist hier besonders wichtig.',
      'Kein Bezug zu Hafen oder Buchstabe. Die Verbindung zu Docklet trägt allein die Wortmarke.',
      'Die schwere Fläche dominiert neben dünner Schrift. Deshalb ist die Wortmarke kräftig gesetzt.',
    ],
    mark: {
      regular: [
        path('tile', 'M10 4 H17 A1 1 0 0 1 18 5 V12 A2 2 0 0 0 20 14 H27 A1 1 0 0 1 28 15 V22 A6 6 0 0 1 22 28 H10 A6 6 0 0 1 4 22 V10 A6 6 0 0 1 10 4 Z', 'base'),
        rect('module', 20, 4, 8, 8, 2.25),
      ],
      small: [
        path('tile', 'M4 1 H8.5 A0.5 0.5 0 0 1 9 1.5 V6 A1 1 0 0 0 10 7 H14.5 A0.5 0.5 0 0 1 15 7.5 V12 A3 3 0 0 1 12 15 H4 A3 3 0 0 1 1 12 V4 A3 3 0 0 1 4 1 Z', 'base'),
        rect('module', 10, 1, 5, 5, 1.1),
      ],
      ink: { regular: [4, 4, 28, 28], small: [1, 1, 15, 15] },
      guides: [{ h: 4 }, { h: 28 }, { v: 4 }, { v: 28 }, { v: 18 }, { h: 14 }, { line: [29, 3, 15, 17] }],
    },
    wordmark: {
      set: 'lowerRound', main: 'docklet', next: 'NEXT', nextSet: 'upperSquare', placement: 'inline',
      weight: 3, nextWeight: 3.2, tracking: 5, nextTracking: 6.5, space: 11, scale: 1.14, nextScale: 0.76,
      linecap: 'round', linejoin: 'round', nextLinecap: 'square', nextLinejoin: 'miter', gap: 7.5, align: { baseline: 28 },
      compact: { scale: 1, nextScale: 0.5, nextWeight: 3.8, nextTracking: 8, lineGap: 5, center: 16 },
    },
    motion: {
      duration: 1200,
      markEnd: 0.72,
      parts: [
        {
          part: 'tile', from: 0, to: 0.36, origin: '50% 50%', label: 'Kachel spannt sich aus der Mitte auf',
          frames: [
            { offset: 0, opacity: 0, transform: 'scale(0.62)', easing: OUT },
            { offset: 0.3, opacity: 1 },
            { offset: 0.72, transform: 'scale(1.025)', easing: IN_OUT },
            { offset: 1, opacity: 1, transform: 'none' },
          ],
        },
        {
          part: 'module', from: 0.24, to: 0.66, origin: '50% 50%', label: 'Modul wächst diagonal in die Ecke und rastet ein',
          frames: [
            { offset: 0, opacity: 0, transform: 'translate(2.5px, -2.5px) scale(0.5)', easing: OUT },
            { offset: 0.3, opacity: 1 },
            { offset: 0.74, transform: 'translate(-0.6px, 0.6px) scale(1.03)', easing: IN_OUT },
            { offset: 1, opacity: 1, transform: 'none' },
          ],
        },
        { part: 'base', from: 0.54, to: 0.72, origin: '50% 50%', label: 'Kachel gibt beim Einrasten kurz nach', frames: settle(0.975) },
      ],
      letters: { kind: 'pop', from: 0.4, to: 1, span: 0.42, label: 'Buchstaben setzen sich nacheinander ein' },
    },
  },
  {
    id: 'orbit',
    name: 'Orbit',
    summary: 'Ein Modul läuft auf dem Ring um die Nabe und dockt an der offenen Stelle an.',
    idea: 'Ein Ring mit Nabe steht für den Hub, das Modul für einen Agenten auf einem Host. Es dockt dort an, wo der Ring offen ist, wie an einer Raumstation statt im Hafen. Die einzige runde Silhouette der Studie. Die Wortmarke ist zweizeilig und damit von Haus aus kompakt.',
    strengths: [
      'Runde Silhouette, klar verschieden von allen anderen Kandidaten.',
      'Erzählt Hub und Agenten direkt: Docklet verwaltet Hosts, die andocken.',
      'Die Bewegung ist die ruhigste der Studie: Das Modul fährt den eben gezeichneten Ring entlang.',
    ],
    risks: [
      'Ring mit Punkt kann wie Zielscheibe, Radio-Button oder Aktualisieren-Symbol wirken.',
      'Bei 16 px liegen Ring, Nabe und Modul eng beieinander. Der 16er-Master braucht 1-px-Fugen.',
      'Kein Buchstabe. Der Bezug zu Docklet entsteht nur über die Wortmarke.',
    ],
    mark: {
      regular: [
        band('ring', 'M26.24 16.54 A10.25 10.25 0 1 1 15.46 5.76', 3.5, { round: true }),
        circle('hub', 16, 16, 3.5),
        rect('module', 19.25, 4.75, 8, 8, 2),
      ],
      small: [band('ring', 'M13 8 A5 5 0 1 1 8 3', 2, { round: true }), circle('hub', 8, 8, 1.75), rect('module', 10, 1, 5, 5, 1)],
      ink: { regular: [4, 4, 28, 28], small: [2, 1, 15, 14] },
      guides: [{ h: 16 }, { v: 16 }, { circle: [16, 16, 10.25] }, { line: [16, 16, 28.5, 3.5] }, { circle: [16, 16, 3.5] }],
    },
    wordmark: {
      set: 'lowerRound', main: 'docklet', next: 'NEXT', nextSet: 'upperSquare', placement: 'below',
      weight: 2.1, nextWeight: 3.4, tracking: 5, nextTracking: 9.5, scale: 0.92, nextScale: 0.43, lineGap: 4.2,
      linecap: 'round', linejoin: 'round', nextLinecap: 'square', nextLinejoin: 'miter', gap: 7, align: { center: 16 },
    },
    motion: {
      duration: 1300,
      markEnd: 0.72,
      parts: [
        { part: 'ring', from: 0, to: 0.42, label: 'Ring zeichnet sich im Uhrzeigersinn', frames: drawOn() },
        { part: 'hub', from: 0.08, to: 0.38, origin: '50% 50%', label: 'Nabe erscheint in der Mitte', frames: popIn(0) },
        {
          // Rotation and scale share the ring centre as origin: the module
          // spirals outwards along the ring and stays inside the frame.
          part: 'module', from: 0.22, to: 0.72, origin: { regular: '-3.25px 11.25px', small: '-2px 7px' }, label: 'Modul fährt den Ring entlang nach außen und dockt an',
          frames: [
            { offset: 0, opacity: 0, transform: 'rotate(-220deg) scale(0.62)', easing: OUT },
            { offset: 0.2, opacity: 1 },
            { offset: 0.8, transform: 'rotate(4deg) scale(1.02)', easing: IN_OUT },
            { offset: 1, opacity: 1, transform: 'none' },
          ],
        },
      ],
      letters: { kind: 'draw', from: 0.46, to: 1, span: 0.45, label: 'Buchstaben zeichnen sich nacheinander' },
    },
  },
  {
    id: 'cradle',
    name: 'Cradle',
    summary: 'Ein Modul setzt sich selbstzentrierend in eine V-förmige Aufnahme.',
    idea: 'Eine V-Aufnahme im rechten Winkel und darüber ein Modul auf der Spitze. Die Kanten liegen parallel, die Fuge ist überall gleich. Vorbild sind selbstzentrierende Aufnahmen und Andockführungen. Das Thema ist präzises, geführtes Andocken. Die einzige Silhouette, die nach unten zeigt.',
    strengths: [
      'Eigenständige Silhouette (Raute über V), auch klein gut wiedererkennbar.',
      'Symmetrisch. Wirkt im gestapelten Logo und als App-Icon ausgewogen.',
      'Saubere Geometrie mit einem Winkel und einer Fuge. Eignet sich besonders gut für den Blueprint-Aufbau.',
    ],
    risks: [
      'Das V kann als Aufklappen- oder Download-Pfeil gelesen werden, die Raute als Edelstein.',
      'Die Diagonalen liegen nicht auf dem Pixelraster und werden bei 16 px weich.',
      'Breiter als hoch. Die Spitze des V liegt optisch tiefer als die Grundlinie der Schrift.',
    ],
    mark: {
      regular: [band('dock', CRADLE.dock, 4.2, { round: true, group: 'cradle' }), path('gem', CRADLE.gem)],
      small: [band('dock', CRADLE_SMALL.dock, 2.2, { round: true, group: 'cradle' }), path('gem', CRADLE_SMALL.gem)],
      ink: { regular: [3.4, 5.04, 28.6, 27.3], small: [1.7, 2.12, 14.3, 13.7] },
      guides: [{ v: 16 }, { h: 25.2 }, { line: [16, 25.2, 3.2, 12.4] }, { line: [16, 25.2, 28.8, 12.4] }, { h: CRADLE.centerY }, { circle: [16, CRADLE.centerY, 5.5] }],
    },
    wordmark: {
      set: 'lowerRound', main: 'docklet', next: 'next', placement: 'inline',
      weight: 2.5, nextWeight: 2.5, tracking: 4.8, space: 10, scale: 1.12, nextScale: 0.94,
      linecap: 'round', linejoin: 'round', gap: 7, align: { baseline: 26.6 },
      compact: { scale: 0.98, nextScale: 0.62, nextWeight: 3, nextTracking: 8, lineGap: 5.4, center: 16.2 },
    },
    motion: {
      duration: 1200,
      markEnd: 0.7,
      parts: [
        { part: 'dock', from: 0, to: 0.34, label: 'V-Aufnahme zeichnet sich von Arm zu Arm', frames: drawOn(OUT) },
        {
          // Starts as an upright square, so it stays inside the frame while it falls.
          part: 'gem', from: 0.2, to: 0.66, origin: '50% 50%', label: 'Modul fällt als Quadrat ein und dreht sich in die Aufnahme',
          frames: [
            { offset: 0, opacity: 0, transform: 'translateY(-6px) rotate(-45deg)', easing: FALL },
            { offset: 0.2, opacity: 1 },
            { offset: 0.6, transform: 'translateY(0.6px) rotate(5deg)', easing: OUT },
            { offset: 0.84, transform: 'translateY(-0.3px) rotate(-1deg)', easing: IN_OUT },
            { offset: 1, opacity: 1, transform: 'none' },
          ],
        },
        { part: 'cradle', from: 0.5, to: 0.7, origin: '50% 0%', label: 'Aufnahme federt beim Aufsetzen', frames: settle(0.95) },
      ],
      letters: { kind: 'rise', from: 0.42, to: 1, span: 0.42, label: 'Buchstaben steigen gestaffelt auf' },
    },
  },
];
