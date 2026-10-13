import { useCallback, useLayoutEffect, useMemo, useRef, useState } from "react";

import { parseCompose } from "@/domain/compose/model";
import { ComposeEditError, mutate, type Mutator } from "@/domain/compose/ops";
import { validateCompose } from "@/domain/compose/validate";
import { sameStackLabels } from "@/domain/stack-labels";

export interface DraftSeed {
  compose: string;
  env: string;
  hostId: string;
  labels: string[];
}

export interface ComposeDraft {
  compose: string;
  env: string;
  hostId: string;
  labels: string[];
  parsed: ReturnType<typeof parseCompose>;
  issues: ReturnType<typeof validateCompose>;
  dirty: boolean;
  savedCompose: string;
  composeDirty: boolean;
  envDirty: boolean;
  labelsDirty: boolean;
  setCompose: (text: string) => void;
  setEnv: (text: string) => void;
  setHostId: (id: string) => void;
  setLabels: (labels: string[]) => void;
  /** Applies a form edit; returns the error code instead of throwing. */
  edit: (fn: Mutator) => ComposeEditError | null;
  reset: () => void;
  markSaved: () => void;
}

/**
 * Single synchronized draft for Easy and Advanced. The raw text is the source of
 * truth; the form model is derived and never written back wholesale.
 */
export function useComposeDraft(seed: DraftSeed): ComposeDraft {
  const [saved, setSaved] = useState(seed);
  const [compose, setComposeState] = useState(seed.compose);
  const [env, setEnv] = useState(seed.env);
  const [hostId, setHostId] = useState(seed.hostId);
  const [labels, setLabels] = useState(seed.labels);
  // Tracks the newest text for back-to-back form edits within one event.
  const latest = useRef(seed.compose);
  const setCompose = useCallback((text: string) => {
    latest.current = text;
    setComposeState(text);
  }, []);

  useLayoutEffect(() => {
    latest.current = compose;
  }, [compose]);

  // Adopt newer server files (after deploys or updates) while nothing is edited locally.
  const [seenSeed, setSeenSeed] = useState(seed);
  if (seed.compose !== seenSeed.compose || seed.env !== seenSeed.env || !sameStackLabels(seed.labels, seenSeed.labels)) {
    setSeenSeed(seed);
    const pristine = compose === saved.compose && env === saved.env && sameStackLabels(labels, saved.labels);
    if (pristine) {
      setSaved(seed);
      setComposeState(seed.compose);
      setEnv(seed.env);
      setLabels(seed.labels);
    }
  }

  const parsed = useMemo(() => parseCompose(compose), [compose]);
  const issues = useMemo(() => (parsed.ok ? validateCompose(parsed.model) : []), [parsed]);

  const edit = useCallback((fn: Mutator) => {
    try {
      setCompose(mutate(latest.current, fn));
      return null;
    } catch (e) {
      if (e instanceof ComposeEditError) return e;
      throw e;
    }
  }, [setCompose]);

  const composeDirty = compose !== saved.compose;
  const envDirty = env !== saved.env;
  const labelsDirty = !sameStackLabels(labels, saved.labels);

  return {
    compose,
    env,
    hostId,
    labels,
    parsed,
    issues,
    savedCompose: saved.compose,
    composeDirty,
    envDirty,
    labelsDirty,
    dirty: composeDirty || envDirty || labelsDirty,
    setCompose,
    setEnv,
    setHostId,
    setLabels,
    edit,
    reset: () => {
      setCompose(saved.compose);
      setEnv(saved.env);
      setHostId(saved.hostId);
      setLabels(saved.labels);
    },
    markSaved: () => setSaved({ compose, env, hostId, labels }),
  };
}
