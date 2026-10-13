import { Terminal, Wand2 } from "lucide-react";
import { useMemo, useState } from "react";
import { stringify } from "yaml";
import { useTranslations } from "use-intl";

import { convertDockerRun, DockerRunError } from "@/domain/compose/docker-run";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldLabel } from "@/shared/ui/field";
import { Textarea } from "@/shared/ui/textarea";

const EXAMPLE = `docker run -d --name uptime -p 3005:3001 \\
  -v kuma_data:/app/data -e TZ=Europe/Berlin -e ADMIN_TOKEN \\
  --restart unless-stopped louislam/uptime-kuma:1.23.16`;

interface DockerRunDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  existing: string[];
  onAdd: (key: string, body: Record<string, unknown>) => void;
}

/** Converts a docker run command into a Compose service; lists unsupported flags. */
export function DockerRunDialog({ open, onOpenChange, existing, onAdd }: DockerRunDialogProps) {
  const t = useTranslations("editor.run");
  const tc = useTranslations("common.actions");
  const [input, setInput] = useState("");
  const result = useMemo(() => {
    if (!input.trim()) return null;
    try {
      return { ok: true as const, conv: convertDockerRun(input, existing) };
    } catch (e) {
      if (e instanceof DockerRunError) return { ok: false as const };
      throw e;
    }
  }, [input, existing]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Terminal className="size-4 text-primary-text" /> {t("title")}
          </DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        <Field>
          <div className="flex items-center justify-between">
            <FieldLabel htmlFor="run-input">{t("input")}</FieldLabel>
            <Button variant="link" size="sm" className="h-auto p-0 text-xs" onClick={() => setInput(EXAMPLE)}>
              <Wand2 data-icon="inline-start" /> {t("example")}
            </Button>
          </div>
          <Textarea id="run-input" rows={4} spellCheck={false} className="font-mono text-xs" placeholder={t("placeholder")} value={input} onChange={(e) => setInput(e.target.value)} />
        </Field>
        {result && !result.ok && <p className="text-sm text-destructive">{t("notRun")}</p>}
        {result?.ok && (
          <div className="grid gap-3 md:grid-cols-2">
            <div>
              <div className="mb-1 text-xs font-medium text-muted-foreground">{t("preview")}</div>
              <pre className="max-h-56 overflow-auto rounded-xl border bg-code p-3 font-mono text-[11.5px] leading-relaxed">{stringify({ [result.conv.key]: result.conv.body }, { lineWidth: 0 })}</pre>
            </div>
            <div>
              <div className="mb-1 text-xs font-medium text-muted-foreground">{t("hints")}</div>
              <ul className="flex flex-col gap-1.5 text-xs">
                {result.conv.hints.map((h, i) => (
                  <li key={i} className="rounded-lg border border-warning/30 bg-warning/8 px-2.5 py-1.5 text-warning">
                    {t(`hint.${h.code}`, { value: h.value ?? "" })}
                  </li>
                ))}
              </ul>
            </div>
          </div>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {tc("cancel")}
          </Button>
          <Button
            disabled={!result?.ok}
            onClick={() => {
              if (!result?.ok) return;
              onAdd(result.conv.key, result.conv.body);
              setInput("");
              onOpenChange(false);
            }}
          >
            {t("add")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
