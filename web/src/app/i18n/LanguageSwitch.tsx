import { Languages } from "lucide-react";
import { useTranslations } from "use-intl";

import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { cn } from "@/shared/lib/utils";

import { LOCALES, setAppLocale, useAppLocale, type AppLocale } from "./locale-store";

const NAME_KEY: Record<AppLocale, "de" | "enUS"> = { de: "de", "en-US": "enUS" };

/**
 * Interface language for this browser; switches every route at once. Each
 * option names its language in that language, so it is findable without
 * reading the current one.
 */
export function LanguageSwitch({ size = "sm", className }: { size?: "xs" | "sm" | "md"; className?: string }) {
  const t = useTranslations("locale.switcher");
  const locale = useAppLocale();
  return (
    <div className={cn("flex items-center gap-2", className)} data-slot="language-switch">
      <Languages className="size-4 shrink-0 text-muted-foreground" aria-hidden />
      <SegmentedToggle
        value={locale}
        onChange={setAppLocale}
        label={t("label")}
        size={size}
        fit="content"
        options={LOCALES.map((l) => ({ value: l, label: <span lang={l}>{t(`names.${NAME_KEY[l]}`)}</span>, ariaLabel: t(`names.${NAME_KEY[l]}`) }))}
      />
    </div>
  );
}
