import { Check, Copy } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslations } from "use-intl";

import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    // Clipboard API needs a secure context; fall back to a temporary textarea.
    const area = document.createElement("textarea");
    area.value = text;
    area.style.position = "fixed";
    area.style.opacity = "0";
    document.body.append(area);
    area.select();
    const ok = document.execCommand?.("copy") ?? false;
    area.remove();
    return ok;
  }
}

export function CopyButton({ text, label, className, variant = "outline" }: { text: string; label?: string; className?: string; variant?: "outline" | "secondary" | "default" }) {
  const t = useTranslations("common.demo");
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  return (
    <Button
      type="button"
      size="sm"
      variant={variant}
      className={cn("h-8", className)}
      onClick={async () => {
        const ok = await copyText(text);
        setCopied(ok);
        clearTimeout(timer.current);
        timer.current = setTimeout(() => setCopied(false), 1800);
      }}
    >
      {copied ? <Check data-icon="inline-start" className="animate-pop-in text-success" /> : <Copy data-icon="inline-start" />}
      <span aria-live="polite">{copied ? t("copied") : (label ?? t("copy"))}</span>
    </Button>
  );
}

/** Scrollable monospaced block with a copy action; content is plain text. */
export function CodeBlock({ code, label, copyLabel, primary, className, maxHeight = "18rem" }: { code: string; label: string; copyLabel?: string; primary?: boolean; className?: string; maxHeight?: string }) {
  return (
    <figure className={cn("overflow-hidden rounded-xl border bg-code", className)}>
      <figcaption className="flex flex-wrap items-center gap-2 border-b bg-muted/30 px-3 py-2 text-xs font-medium text-muted-foreground">
        <span className="mr-auto truncate font-mono">{label}</span>
        <CopyButton text={code} label={copyLabel} variant={primary ? "default" : "outline"} />
      </figcaption>
      <pre tabIndex={0} aria-label={label} className="overflow-auto p-3 font-mono text-[12px] leading-relaxed whitespace-pre outline-none focus-visible:ring-2 focus-visible:ring-ring" style={{ maxHeight }}>
        {code}
      </pre>
    </figure>
  );
}
