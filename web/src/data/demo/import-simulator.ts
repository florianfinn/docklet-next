/*
 * Simulated one-time GitHub/Docker Hub import; no network request is made.
 * Adapted from the Opus Arcane mockup compose.ts simulateImport (copied 2026-10-10).
 */
import { stringify } from "yaml";
import type { ImportRequest, ImportResult } from "../adapter";
import { AdapterOutcomeError } from "../capability";

export function simulateImport({ kind, ref }: ImportRequest): ImportResult {
  if (!ref.trim()) throw new AdapterOutcomeError({ kind: "error", code: "import.invalid-reference" });
  const title = (ref.split("/").pop() ?? "app").split(":")[0] || "app";
  const github = kind === "github";
  const envNames = github ? ["DATABASE_URL", "POSTGRES_PASSWORD"] : [];
  return { title, composeContent: stringify({ services: { [title]: { image: github ? "ghcr.io/example/notes-app:1.4.0" : ref.includes(":") ? ref : `${ref}:latest`, restart: "unless-stopped", ...(github ? { environment: { DATABASE_URL: "${DATABASE_URL}", POSTGRES_PASSWORD: "${POSTGRES_PASSWORD}" } } : {}) } } }), envNames,
    found: github ? ["import.found.compose", "import.found.env-example"] : ["import.found.image-config"], missing: github ? ["import.missing.variables"] : [] };
}
