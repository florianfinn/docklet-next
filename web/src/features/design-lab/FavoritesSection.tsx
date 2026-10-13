import { ClipboardCopy, Code2, Download, Package, RotateCcw, Star, Trash2 } from "lucide-react";
import { useId, useState } from "react";
import { toast } from "sonner";
import { useFormatter, useTranslations } from "use-intl";

import { Button } from "@/shared/ui/button";
import { Field, FieldDescription, FieldError, FieldLabel, FieldLegend, FieldSet } from "@/shared/ui/field";
import { Textarea } from "@/shared/ui/textarea";

import { applySnapshot, captureSnapshot, clearFavorite, decodeDesignCode, encodeDesignCode, resetToFavorite, resetToShipped, saveFavorite, SHIPPED_DEFAULT_CODE, useFavorite, type DecodeError } from "./favorite-code";

/**
 * Design code export and import plus the local favourite. Lets a user send
 * the look they settled on; the code holds presentation settings only.
 */
export function FavoritesSection() {
  const t = useTranslations("visual.favorites");
  const format = useFormatter();
  const favorite = useFavorite();
  const [code, setCode] = useState("");
  const [input, setInput] = useState("");
  const [error, setError] = useState<DecodeError | null>(null);
  const exportId = useId();
  const importId = useId();
  const errorId = useId();

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      toast.success(t("copied"));
    } catch {
      toast.error(t("copyFailed"));
    }
  };

  const apply = () => {
    const result = decodeDesignCode(input);
    if (!result.ok) {
      setError(result.error);
      return;
    }
    setError(null);
    const applied = applySnapshot(result.snapshot);
    toast.success(t("applied"), { description: t("sections", { count: applied.length }) });
  };

  return (
    <FieldSet data-slot="design-favorites">
      <FieldLegend variant="label">{t("title")}</FieldLegend>
      <p className="-mt-2 text-xs text-muted-foreground">{t("hint")}</p>

      <Field>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" onClick={() => setCode(encodeDesignCode(captureSnapshot()))}>
            <Code2 data-icon="inline-start" />
            {t("export")}
          </Button>
          <Button variant="outline" size="sm" disabled={!code} onClick={() => void copy()}>
            <ClipboardCopy data-icon="inline-start" />
            {t("copy")}
          </Button>
        </div>
        {code && (
          <>
            <FieldLabel htmlFor={exportId} className="sr-only">
              {t("codeLabel")}
            </FieldLabel>
            <Textarea id={exportId} readOnly value={code} rows={3} className="font-mono text-[11px] break-all" onFocus={(e) => e.currentTarget.select()} data-slot="design-code" />
          </>
        )}
      </Field>

      <Field data-invalid={error ? true : undefined}>
        <FieldLabel htmlFor={importId}>{t("importLabel")}</FieldLabel>
        <Textarea
          id={importId}
          value={input}
          rows={3}
          placeholder={t("importPlaceholder")}
          className="font-mono text-[11px] break-all"
          aria-invalid={error ? true : undefined}
          aria-describedby={error ? errorId : undefined}
          onChange={(e) => {
            setInput(e.target.value);
            setError(null);
          }}
        />
        {error && <FieldError id={errorId}>{t(`errors.${error}`)}</FieldError>}
        <Button size="sm" className="self-start" onClick={apply}>
          <Download data-icon="inline-start" />
          {t("apply")}
        </Button>
      </Field>

      <Field>
        <FieldLabel>{t("favorite")}</FieldLabel>
        <FieldDescription>{t("favoriteHint")}</FieldDescription>
        <p className="text-xs text-muted-foreground" data-slot="favorite-state">
          {favorite ? t("savedAt", { time: format.dateTime(new Date(favorite.savedAt), { dateStyle: "short", timeStyle: "short" }) }) : t("noFavorite")}
        </p>
        <div className="flex flex-wrap gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              saveFavorite();
              toast.success(t("savedFavorite"));
            }}
          >
            <Star data-icon="inline-start" />
            {t("saveFavorite")}
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              toast.success(resetToFavorite() === "favorite" ? t("resetDone") : t("resetShippedDone"));
            }}
          >
            <RotateCcw data-icon="inline-start" />
            {t("resetFavorite")}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            disabled={!favorite}
            onClick={() => {
              clearFavorite();
              toast(t("clearedFavorite"));
            }}
          >
            <Trash2 data-icon="inline-start" />
            {t("clearFavorite")}
          </Button>
        </div>
      </Field>

      <Field>
        <FieldLabel>{t("shipped")}</FieldLabel>
        <FieldDescription>{t("shippedHint")}</FieldDescription>
        <code className="text-[11px] break-all text-muted-foreground" data-slot="shipped-code">
          {SHIPPED_DEFAULT_CODE.slice(0, 40)}…
        </code>
        <Button
          variant="outline"
          size="sm"
          className="self-start"
          onClick={() => {
            resetToShipped();
            toast.success(t("resetShippedDone"));
          }}
        >
          <Package data-icon="inline-start" />
          {t("resetShipped")}
        </Button>
      </Field>
    </FieldSet>
  );
}
