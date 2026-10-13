import { Check, CircleAlert, Rows3, Rows4 } from "lucide-react";
import { useState, type ReactNode } from "react";
import { useTranslations } from "use-intl";

import { formatNumber } from "@/domain/locale";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { ToneBadge, ToneDot } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Input } from "@/shared/ui/input";
import { Slider } from "@/shared/ui/slider";
import { Switch } from "@/shared/ui/switch";

import { normalizeHex } from "./color";
import { matchPreset, PALETTE_PRESETS, PALETTE_ROLES, paletteTokens, type Palette, type PaletteRole } from "./palette";

interface PaletteEditorProps {
  value: Palette;
  onChange: (palette: Palette) => void;
  theme: "dark" | "light";
  idPrefix: string;
}

const fmt = (n: number) => formatNumber(n, { maximumFractionDigits: 1, minimumFractionDigits: 1 });

/**
 * Curated presets plus three freely chosen colour roles with contrast readouts
 * and a small preview of real controls. The caller decides whether changes
 * apply at once (design lab) or as a reversible preview (settings).
 */
export function PaletteEditor({ value, onChange, theme, idPrefix }: PaletteEditorProps) {
  const t = useTranslations("theme");
  const preset = matchPreset(value);
  const tokens = paletteTokens(value, theme);
  const set = (role: PaletteRole, hex: string) => onChange({ ...value, [role]: hex });

  return (
    <div className="@container flex flex-col gap-5" data-slot="palette-editor">
      <fieldset className="flex flex-col gap-2">
        <legend className="mb-2 text-sm font-medium">{t("presets.label")}</legend>
        <div
          role="radiogroup"
          aria-label={t("presets.label")}
          className="grid gap-2 @lg:grid-cols-2"
          onKeyDown={(e) => {
            const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
            if (!step) return;
            e.preventDefault();
            const radios = [...e.currentTarget.querySelectorAll<HTMLButtonElement>('[role="radio"]')];
            const index = radios.indexOf(document.activeElement as HTMLButtonElement);
            const next = (index + step + radios.length) % radios.length;
            const target = PALETTE_PRESETS[next];
            if (target) onChange(target.palette);
            radios[next]?.focus();
          }}
        >
          {PALETTE_PRESETS.map((p, i) => {
            const active = p.id === preset;
            return (
              <button
                key={p.id}
                type="button"
                role="radio"
                aria-checked={active}
                tabIndex={active || (!preset && i === 0) ? 0 : -1}
                onClick={() => onChange(p.palette)}
                className={cn(
                  "lift flex items-start gap-3 rounded-xl border bg-card/60 p-3 text-left outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
                  active && "border-primary/60 bg-primary/10",
                )}
              >
                <span className="mt-0.5 flex shrink-0 -space-x-1.5" aria-hidden>
                  {PALETTE_ROLES.map((r) => (
                    <i key={r} className="size-5 rounded-full border-2 border-card" style={{ background: p.palette[r] }} />
                  ))}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="flex items-center gap-1.5 text-sm font-medium">
                    {t(`presets.names.${p.id}`)}
                    {active && <Check className="size-3.5 text-primary-text" aria-hidden />}
                  </span>
                  <span className="block text-xs text-muted-foreground">{t(`presets.descriptions.${p.id}`)}</span>
                </span>
              </button>
            );
          })}
        </div>
        <p className="text-xs text-muted-foreground" aria-live="polite">
          {preset ? t("presets.active", { name: t(`presets.names.${preset}`) }) : `${t("presets.custom")} – ${t("presets.customHint")}`}
        </p>
      </fieldset>

      <fieldset className="flex flex-col gap-3">
        <legend className="mb-1 text-sm font-medium">{t("roles.label")}</legend>
        <p className="-mt-1 text-xs text-muted-foreground">{t("roles.hint")}</p>
        {PALETTE_ROLES.map((role) => (
          <RoleRow key={role} id={`${idPrefix}-${role}`} role={role} value={value[role]} onChange={(hex) => set(role, hex)} readout={<Readout role={role} tokens={tokens} />} />
        ))}
      </fieldset>

      <PalettePreview palette={value} />
    </div>
  );
}

function RoleRow({ id, role, value, onChange, readout }: { id: string; role: PaletteRole; value: string; onChange: (hex: string) => void; readout: ReactNode }) {
  const t = useTranslations("theme.roles");
  const [text, setText] = useState(value);
  // Picker, preset or outside changes replace the text unless it already means the same colour.
  const [seen, setSeen] = useState(value);
  if (value !== seen) {
    setSeen(value);
    if (normalizeHex(text) !== value) setText(value);
  }
  const parsed = normalizeHex(text);
  const name = t(`${role}.name`);
  return (
    <div className="grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-3 gap-y-1.5 rounded-xl border bg-card/40 p-3" data-role={role}>
      <input
        type="color"
        aria-label={t("picker", { role: name })}
        value={value}
        onChange={(e) => onChange(e.target.value.toLowerCase())}
        className="size-10 cursor-pointer rounded-lg border bg-transparent p-0.5 [&::-webkit-color-swatch]:rounded-md [&::-webkit-color-swatch]:border-0 [&::-webkit-color-swatch-wrapper]:p-0"
      />
      <div className="min-w-0">
        <label htmlFor={id} className="text-sm font-medium">
          {name}
        </label>
        <p className="text-xs text-muted-foreground">{t(`${role}.applies`)}</p>
      </div>
      <div className="col-span-2 flex flex-wrap items-center gap-2 @sm:col-span-1 @sm:col-start-2">
        <Input
          id={id}
          value={text}
          spellCheck={false}
          aria-label={t("hex", { role: name })}
          aria-invalid={!parsed || undefined}
          aria-describedby={parsed ? undefined : `${id}-err`}
          onChange={(e) => {
            setText(e.target.value);
            const hex = normalizeHex(e.target.value);
            if (hex) onChange(hex);
          }}
          onBlur={() => parsed && setText(parsed)}
          className="h-8 w-28 font-mono text-xs uppercase"
        />
        {readout}
        {!parsed && (
          <span id={`${id}-err`} className="text-xs text-destructive">
            {t("invalid")}
          </span>
        )}
      </div>
    </div>
  );
}

function Ratio({ label, value, min = 4.5 }: { label: string; value: number; min?: number }) {
  const t = useTranslations("theme.readout");
  const ok = value >= min;
  return (
    <span className={cn("inline-flex items-center gap-1 rounded-md border px-1.5 py-0.5 text-[11px] tabular", ok ? "text-muted-foreground" : "border-warning/40 text-warning")}>
      {!ok && <CircleAlert className="size-3" aria-hidden />}
      {label} {t("ratio", { value: fmt(value) })}
      {!ok && <span className="sr-only">, {t("low", { min: fmt(min) })}</span>}
    </span>
  );
}

function Readout({ role, tokens }: { role: PaletteRole; tokens: ReturnType<typeof paletteTokens> }) {
  const t = useTranslations("theme.readout");
  const r = tokens.report;
  if (role === "primary")
    return (
      <>
        <Ratio label={t("label")} value={r.primaryLabel} />
        <Ratio label={t("text")} value={r.primaryText} />
        <Ratio label={t("secondary")} value={r.secondaryLabel} />
        <Ratio label={t("control")} value={r.controlContrast} min={3} />
      </>
    );
  if (role === "secondary")
    return (
      <>
        <Ratio label={t("text")} value={r.secondaryText} />
        <Ratio label={t("highlight")} value={r.highlightContrast} min={3} />
      </>
    );
  return <span className="font-mono text-[11px] text-muted-foreground uppercase">{t("surfaces", tokens.surfaces)}</span>;
}

/** Real controls in the current document colours; they follow every palette change live. */
function PalettePreview({ palette }: { palette: Palette }) {
  const t = useTranslations("theme.preview");
  const tr = useTranslations("theme.roles");
  const [mode, setMode] = useState<"compact" | "advanced">("compact");
  const [on, setOn] = useState(true);
  const [level, setLevel] = useState(60);
  const [checked, setChecked] = useState(true);
  return (
    <section aria-label={t("label")} className="flex flex-col gap-3">
      <h3 className="text-sm font-medium">{t("label")}</h3>
      <ul aria-label={t("strip")} className="grid grid-cols-3 overflow-hidden rounded-xl border">
        {PALETTE_ROLES.map((role) => (
          <li key={role} className="flex flex-col">
            <span className="h-10" style={{ background: palette[role] }} aria-hidden />
            <span className="truncate px-2 pt-1.5 text-[11px] font-medium">{tr(`${role}.name`)}</span>
            <span className="px-2 pb-1.5 font-mono text-[10.5px] text-muted-foreground uppercase">{palette[role]}</span>
          </li>
        ))}
      </ul>
      <div className="flex flex-col gap-3 rounded-xl border bg-background p-3">
        <h4 className="text-xs font-medium text-muted-foreground">{t("primaryGroup")}</h4>
        <div className="grid gap-1 rounded-lg bg-sidebar p-1.5 text-sm" aria-hidden>
          <span className="rounded-md bg-primary/12 px-2.5 py-1.5 font-medium text-primary-text">{t("nav")}</span>
          <span data-preview="nav-hover" className="rounded-md bg-sidebar-accent px-2.5 py-1.5 text-sidebar-accent-foreground">{t("navHover")}</span>
          <span className="rounded-md px-2.5 py-1.5 text-muted-foreground">{t("navOther")}</span>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Button size="sm" type="button">
            {t("primary")}
          </Button>
          <Button size="sm" type="button" variant="secondary" data-preview="secondary">
            {t("secondary")}
          </Button>
          <Button size="sm" type="button" variant="outline">
            {t("outline")}
          </Button>
          <span className="mx-1 h-5 w-px bg-border" aria-hidden />
          <span data-preview="secondary-hover" className="inline-flex h-8 items-center rounded-md bg-secondary-hover px-3 text-sm font-medium text-secondary-foreground" aria-hidden>
            {t("secondaryHover")}
          </span>
          <span data-preview="ghost-hover" className="inline-flex h-8 items-center rounded-md bg-accent px-3 text-sm font-medium text-accent-foreground" aria-hidden>
            {t("ghostHover")}
          </span>
        </div>
        <p className="-mt-1 text-[11px] text-muted-foreground">{t("hoverNote")}</p>
        <div className="flex flex-wrap items-center gap-4">
          <SegmentedToggle
            value={mode}
            onChange={setMode}
            label={t("toggle")}
            options={[
              { value: "compact", label: t("compact"), icon: <Rows3 /> },
              { value: "advanced", label: t("advanced"), icon: <Rows4 /> },
            ]}
          />
          <Switch checked={on} onCheckedChange={setOn} aria-label={t("switch")} />
          <Checkbox checked={checked} onCheckedChange={(v) => setChecked(v === true)} aria-label={t("checkbox")} />
        </div>
        <Slider value={[level]} min={0} max={100} step={1} onValueChange={([v]) => setLevel(v ?? level)} aria-label={t("slider")} className="max-w-64" />
        <p className="-mt-1 text-[11px] text-muted-foreground">{t("controlsNote")}</p>
        <h4 className="mt-1 border-t pt-3 text-xs font-medium text-muted-foreground">{t("secondaryGroup")}</h4>
        <div className="flex flex-wrap items-center gap-3" aria-hidden>
          <ToneBadge tone="accent">{t("badge")}</ToneBadge>
          <span data-preview="highlight" className="inline-flex h-8 items-center rounded-md border border-highlight/60 px-3 text-sm text-foreground shadow-[0_0_calc(14px*var(--glow,1))_-2px] shadow-highlight/55">
            {t("glow")}
          </span>
          <span className="inline-flex items-center gap-2 text-xs text-muted-foreground">
            <span data-preview="skeleton" className="h-3 w-16 rounded-md bg-skeleton" />
            {t("skeleton")}
          </span>
          <span className="inline-flex items-center gap-2 text-xs text-muted-foreground">
            <span className="h-3 w-8 rounded-sm bg-chart-1" />
            {t("chart")}
          </span>
        </div>
        <p className="flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
          {t("status")}
          <span className="inline-flex items-center gap-1.5">
            <ToneDot tone="ok" /> {t("ok")}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <ToneDot tone="warn" /> {t("warn")}
          </span>
          <span className="inline-flex items-center gap-1.5">
            <ToneDot tone="err" /> {t("err")}
          </span>
        </p>
      </div>
    </section>
  );
}
