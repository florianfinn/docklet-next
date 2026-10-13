import { X } from "lucide-react";
import { useState, type ComponentProps } from "react";

import { labelTone } from "@/domain/hosts";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";

/** Input that commits on blur or Enter; used for keys where partial values would collide. */
export function CommitInput({ value, onCommit, ...props }: Omit<ComponentProps<typeof Input>, "value" | "onChange"> & { value: string; onCommit: (v: string) => void }) {
  const [local, setLocal] = useState(value);
  const [seen, setSeen] = useState(value);
  if (seen !== value) {
    setSeen(value);
    setLocal(value);
  }
  return (
    <Input
      {...props}
      value={local}
      onChange={(e) => setLocal(e.target.value)}
      onBlur={() => local !== value && onCommit(local)}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          if (local !== value) onCommit(local);
        }
        if (e.key === "Escape") setLocal(value);
      }}
    />
  );
}

export function LabelEditor({ labels, onChange, suggestions, placeholder, addLabel, removeLabel, emptyText }: { labels: string[]; onChange: (labels: string[]) => void; suggestions: string[]; placeholder: string; addLabel: string; removeLabel: (l: string) => string; emptyText: string }) {
  const [input, setInput] = useState("");
  const add = (raw: string) => {
    const v = raw.trim().toLowerCase();
    if (!v || labels.includes(v)) return;
    onChange([...labels, v]);
    setInput("");
  };
  const rest = suggestions.filter((s) => !labels.includes(s)).slice(0, 6);
  return (
    <div className="flex flex-col gap-2">
      <div className="flex min-h-7 flex-wrap items-center gap-1.5">
        {labels.length === 0 && <span className="text-xs text-muted-foreground">{emptyText}</span>}
        {labels.map((l) => (
          <ToneBadge key={l} tone={labelTone(l)} className="animate-pop-in gap-1 pr-1">
            {l}
            <button type="button" onClick={() => onChange(labels.filter((x) => x !== l))} aria-label={removeLabel(l)} className="rounded p-0.5 hover:bg-(--tone)/20">
              <X className="size-3" />
            </button>
          </ToneBadge>
        ))}
      </div>
      <form
        className="flex flex-wrap items-center gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          add(input);
        }}
      >
        <Input value={input} onChange={(e) => setInput(e.target.value)} placeholder={placeholder} aria-label={addLabel} className="h-8 w-44" />
        <Button type="submit" size="sm" variant="outline" disabled={!input.trim()}>
          {addLabel}
        </Button>
        {rest.map((s) => (
          <button key={s} type="button" onClick={() => add(s)} data-tone={labelTone(s)} className="rounded-lg border border-dashed px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:border-(--tone)/50 hover:text-(--tone)">
            + {s}
          </button>
        ))}
      </form>
    </div>
  );
}
