import { useTranslations } from "use-intl";

import type { DraftErrors } from "./onboarding-draft";

/** Resolves a draft validation key to its message once errors are shown. */
export function useFieldError(errors: DraftErrors, show: boolean) {
  const t = useTranslations("hosts.validation");
  return (key: keyof DraftErrors) => {
    const e = errors[key];
    return show && e ? t(e as "required") : undefined;
  };
}
