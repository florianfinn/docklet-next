import { useTranslations } from "use-intl";

import { useTokens } from "@/features/design-lab/use-design-lab";
import { Toaster } from "@/shared/ui/sonner";

import { useToasterPointerFocus } from "./layer-coordination";
import { useNoticeLifetime } from "./notice-life";
import { TOAST_MOBILE_OFFSET, TOAST_OFFSET } from "./toast-corner";

/** The app's toaster in the fixed top-right corner, above non-modal side sheets (action centre, design lab, inspector). */
export function AppToaster() {
  const t = useTranslations("activity.notices");
  const tokens = useTokens();
  useNoticeLifetime();
  useToasterPointerFocus();
  return (
    <Toaster
      position="top-right"
      offset={TOAST_OFFSET}
      mobileOffset={TOAST_MOBILE_OFFSET}
      visibleToasts={5}
      theme={tokens.theme}
      gap={10}
      customAriaLabel={t("region")}
      toastOptions={{ closeButtonAriaLabel: t("close") }}
    />
  );
}
