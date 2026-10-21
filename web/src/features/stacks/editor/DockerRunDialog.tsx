import { Terminal, Wand2 } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { stringify } from "yaml";
import { useTranslations } from "use-intl";

import { AdapterOutcomeError } from "@/data/capability";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { convertDockerRun, DockerRunError } from "@/domain/compose/docker-run";
import { hubConversion, type Declarations, type HubResult } from "./hub-conversion";
import { Spinner } from "@/shared/ui/spinner";
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
  hostId: string;
  onAdd: (key: string, body: Record<string, unknown>, env: string, declarations: Declarations) => void;
}

type Conversion = { id: number; hostId: string; input: string; adapter: unknown } & ({ phase: "pending" } | { phase: "done"; result: HubResult });

/** Converts a docker run command into a Compose service; lists unsupported flags. */
export function DockerRunDialog({ open, onOpenChange, existing, hostId, onAdd }: DockerRunDialogProps) {
  const t = useTranslations("editor.run");
  const tc = useTranslations("common.actions");
  const adapter = useAdapter();
  // Production converts on the hub; the demo keeps the local converter with its notes.
  const viaHub = adapter.kind !== "demo";
  const [input, setInput] = useState("");
  const capability = useCapability("convertDockerRun", { kind: "host", id: hostId });
  // One conversion lifetime: a request settles only while it is still the current one.
  // Closing, editing the command and changing host or adapter end the lifetime.
  const [conversion, setConversion] = useState<Conversion | null>(null);
  const sequence = useRef(0);
  if (conversion && (!open || conversion.hostId !== hostId || conversion.adapter !== adapter)) setConversion(null);
  const converting = conversion?.phase === "pending";
  const local = useMemo(() => {
    if (viaHub || !input.trim()) return null;
    try {
      return { ok: true as const, conv: convertDockerRun(input, existing) };
    } catch (e) {
      if (e instanceof DockerRunError) return { ok: false as const };
      throw e;
    }
  }, [input, existing, viaHub]);
  const result = viaHub ? (conversion?.phase === "done" ? conversion.result : null) : local;
  const change = (value: string) => { setInput(value); setConversion(null); };
  const convert = async () => {
    const id = ++sequence.current;
    const target = { id, hostId, input, adapter };
    setConversion({ ...target, phase: "pending" });
    let next: HubResult;
    try {
      const out = await adapter.convertDockerRun(target.hostId, target.input);
      next = hubConversion(out.compose, out.env, out.serviceName, existing);
    } catch (e) {
      next = { ok: false, reason: e instanceof AdapterOutcomeError && e.outcome.kind === "error" && e.outcome.code === "stack.convert-invalid" ? "invalid" : "failed" };
    }
    setConversion((current) => current?.phase === "pending" && current.id === id ? { ...current, phase: "done", result: next } : current);
  };

  return (
    <Dialog open={open} onOpenChange={(next) => { if (!next) setConversion(null); onOpenChange(next); }}>
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
            <Button variant="link" size="sm" className="h-auto p-0 text-xs" onClick={() => change(EXAMPLE)}>
              <Wand2 data-icon="inline-start" /> {t("example")}
            </Button>
          </div>
          <Textarea id="run-input" rows={4} spellCheck={false} className="font-mono text-xs" placeholder={t("placeholder")} value={input} onChange={(e) => change(e.target.value)} />
        </Field>
        {result && !result.ok && <p role="alert" className="text-sm text-destructive">{"reason" in result && result.reason === "failed" ? t("hubFailed") : t("notRun")}</p>}
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
          <Button variant="ghost" onClick={() => { setConversion(null); onOpenChange(false); }}>
            {tc("cancel")}
          </Button>
          {viaHub && (
            <Button variant="outline" disabled={!input.trim() || converting || !capability.available} title={capability.reason} onClick={() => void convert()}>
              {converting && <Spinner data-icon="inline-start" />} {t("convert")}
            </Button>
          )}
          <Button
            disabled={!result?.ok || converting}
            onClick={() => {
              if (!result?.ok || converting) return;
              onAdd(result.conv.key, result.conv.body, "env" in result && typeof result.env === "string" ? result.env : "", "declarations" in result ? result.declarations as Declarations : {});
              change("");
              setConversion(null);
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
