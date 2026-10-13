import type { AuthMode, AuthSettings } from "@/domain/settings";

/**
 * Login demo contract. It mirrors what a future adapter call would accept and
 * return, but nothing here authenticates: the password is never compared,
 * stored or sent, and a successful demo login sets no session.
 */
export type LoginVariant = AuthMode;
/** Variant from the hub settings, or a fixed one chosen in the demo controls. */
export type VariantChoice = "settings" | LoginVariant;
export type LoginFailure = "invalid" | "unavailable" | "locked";
/** Developer control: forces the next attempt to fail with this reason. */
export type ForcedOutcome = "none" | LoginFailure;

export interface LocalLogin {
  username: string;
  password: string;
}

export type LoginErrors = Partial<Record<keyof LocalLogin, "required">>;
export type LoginResult = { ok: true; user: string } | { ok: false; reason: LoginFailure };

export const LOGIN_DELAY_MS = 700;

export function validateLogin(input: LocalLogin): LoginErrors {
  const errors: LoginErrors = {};
  if (!input.username.trim()) errors.username = "required";
  if (!input.password) errors.password = "required";
  return errors;
}

const CHOICES: readonly VariantChoice[] = ["settings", "local", "proxy", "none"];

/** Demo variant from the URL (?variant=); anything unknown follows the settings. */
export function parseVariantChoice(raw: string | null): VariantChoice {
  return CHOICES.includes(raw as VariantChoice) ? (raw as VariantChoice) : "settings";
}

export function resolveVariant(choice: VariantChoice, auth: AuthSettings | undefined): LoginVariant {
  return choice === "settings" ? (auth?.mode ?? "local") : choice;
}

/**
 * Simulated check after a short delay. Local: the user name must match the
 * configured admin (the hub only knows that a password exists, so any
 * non-empty password passes). Proxy: the trusted proxy is assumed to have sent
 * its user header. A forced outcome fails first; abort rejects silently.
 */
export function simulateLogin(
  variant: LoginVariant,
  input: LocalLogin,
  options: { auth: AuthSettings | undefined; forced: ForcedOutcome; delayMs?: number; signal?: AbortSignal },
): Promise<LoginResult> {
  const result = (): LoginResult => {
    if (options.forced !== "none") return { ok: false, reason: options.forced };
    if (variant !== "local") return { ok: true, user: "" };
    const expected = options.auth?.local.username ?? "admin";
    return input.username.trim() === expected && input.password.length > 0 ? { ok: true, user: expected } : { ok: false, reason: "invalid" };
  };
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => resolve(result()), options.delayMs ?? LOGIN_DELAY_MS);
    options.signal?.addEventListener("abort", () => {
      clearTimeout(timer);
      reject(new DOMException("aborted", "AbortError"));
    });
  });
}
