// Small helpers shared by the motion plans.

import { letteringFor } from './lettering.js';

/** Letters a variant draws; the initial variants drop the letter the mark replaces. */
export function letterCount(candidate, variant = 'full') {
  const { main, next } = letteringFor(candidate, variant);
  return `${main}${next}`.replace(/\s/g, '').length;
}
