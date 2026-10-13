// Registry of the Docklet Next logo candidates and the shared letter
// animations. Candidate data lives in candidates-first.js (Berth, Stack,
// Forward) and candidates-second.js (Slot, Orbit, Cradle).

import { FIRST_ROUND } from './candidates-first.js';
import { SECOND_ROUND } from './candidates-second.js';
import { STACK_LETTER_KINDS } from './motion-stack.js';
import { IN_OUT, OUT } from './shapes.js';

export const CANDIDATES = [...FIRST_ROUND, ...SECOND_ROUND];

export const CANDIDATE_IDS = CANDIDATES.map((c) => c.id);

export function getCandidate(id) {
  const candidate = CANDIDATES.find((c) => c.id === id);
  if (!candidate) throw new Error(`Unknown logo candidate "${id}"`);
  return candidate;
}

/**
 * Letter animations shared by the candidates; frames are per letter, or a
 * function of the run ('main' | 'next') when the end state differs per run.
 */
export const LETTER_KINDS = {
  draw: {
    selector: ' path',
    frames: [
      { offset: 0, opacity: 0, strokeDasharray: '1 1', strokeDashoffset: 1, easing: OUT },
      { offset: 0.08, opacity: 1 },
      { offset: 1, opacity: 1, strokeDasharray: '1 1', strokeDashoffset: 0 },
    ],
  },
  rise: {
    selector: '',
    origin: '50% 100%',
    frames: [
      { offset: 0, opacity: 0, transform: 'translateY(7px)', easing: OUT },
      { offset: 0.65, opacity: 1, transform: 'translateY(-0.8px)', easing: IN_OUT },
      { offset: 1, opacity: 1, transform: 'none' },
    ],
  },
  slide: {
    selector: '',
    frames: [
      { offset: 0, opacity: 0, transform: 'translateX(-7px)', easing: OUT },
      { offset: 0.7, opacity: 1, transform: 'translateX(0.6px)', easing: IN_OUT },
      { offset: 1, opacity: 1, transform: 'none' },
    ],
  },
  pop: {
    selector: '',
    origin: '50% 70%',
    frames: [
      { offset: 0, opacity: 0, transform: 'scale(0.55)', easing: OUT },
      { offset: 0.6, opacity: 1, transform: 'scale(1.06)', easing: IN_OUT },
      { offset: 1, opacity: 1, transform: 'none' },
    ],
  },
  ...STACK_LETTER_KINDS,
};
