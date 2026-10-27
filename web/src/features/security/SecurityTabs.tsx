import { useMemo, useState, type CSSProperties, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { formatRelative } from "@/domain/format";
import { COLLATOR } from "@/domain/hosts";
import type { Finding, Host, ScannedImage, ScanRun, Severity, Stack } from "@/domain/model";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { HostChip } from "@/shared/components/host-chip";
import { ToneBadge } from "@/shared/components/tone";
import { ColumnFilterRow, ColumnFilterSummary } from "@/shared/components/column-filters";
import { ROW_EDGES, ROW_SURFACE } from "@/shared/components/overview/overview-table";
import { SortableHead } from "@/shared/components/sortable-head";
import { activeFilters, applyColumnFilters, type ColumnFilter, type FilterValues } from "@/shared/lib/column-filter";
import type { SortState } from "@/shared/lib/table-sort";
import { cn } from "@/shared/lib/utils";
import { Card } from "@/shared/ui/card";
import { BareTable, TableBody, TableCell, TableHeader, TableRow } from "@/shared/ui/table";

import { RiskGauge, SevDot, ShareBar, TrendChart } from "./OverviewCharts";
import { SEVERITIES, totalOf } from "./severity";

interface Data {
  images: ScannedImage[];
  findings: Finding[];
  history: ScanRun[];
  stacks: Stack[];
  hosts: Map<string, Host>;
}

const short = (ref: string) => ref.split("/").pop() ?? ref;

export function OverviewTab({ images: allImages, findings, history, stacks, hosts, animate, onFocus }: Data & { animate: boolean; onFocus: (id: string) => void }) {
  const t = useTranslations("security");
  const images = allImages.filter((i) => !i.unavailable);
  const top = useMemo(() => [...findings].sort((a, b) => (b.risk ?? b.cvss) - (a.risk ?? a.cvss)).slice(0, 6), [findings]);
  const totals = SEVERITIES.map((s) => ({ key: s.key, label: t(`severities.${s.key}`), color: s.color, n: images.reduce((n, i) => n + (i.counts[s.key] ?? 0), 0) }));
  const usage = (u: ScannedImage["usage"]) => images.filter((i) => i.usage === u).reduce((n, i) => n + totalOf(i), 0);
  const max = Math.max(1, ...images.map(totalOf));
  const stackName = (id?: string) => stacks.find((s) => s.id === id)?.name;
  if (allImages.length && !images.length) return <p role="status">{t("scanUnavailable")}</p>;
  return (
    <div className="flex flex-col gap-6">
      <section className="grid items-center gap-8 py-2 lg:grid-cols-[minmax(0,1fr)_minmax(0,1.5fr)]">
        <RiskGauge images={images} top={top[0]} onDriver={onFocus} />
        <div className="flex flex-col gap-4">
          <div className="flex items-baseline justify-between">
            <h3 className="text-sm font-medium">{t("trend.title")}</h3>
            <span className="text-xs text-muted-foreground">{t("trend.runs", { count: history.length })}</span>
          </div>
          <TrendChart history={history} animate={animate} />
          <ul className="max-h-32 overflow-y-auto text-xs text-muted-foreground" aria-label={t("trend.label")}>
            {history.map((r, index) => <li key={`${r.hostId}:${r.id ?? index}`}>{hosts.get(r.hostId ?? "")?.name ?? r.hostId} · {formatRelative(r.at)} · {r.status ? t(`historyStatus.${r.status}`) : r.trigger}</li>)}
          </ul>
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <h4 className="mb-2 text-sm text-muted-foreground">{t("share.severity")}</h4>
              <ShareBar parts={totals} />
            </div>
            <div>
              <h4 className="mb-2 text-sm text-muted-foreground">{t("share.exposure")}</h4>
              <ShareBar
                parts={[
                  { key: "running", label: t("share.running"), color: "var(--primary)", n: usage("running") },
                  { key: "stopped", label: t("share.stopped"), color: "color-mix(in oklch, var(--primary) 50%, transparent)", n: usage("stopped") },
                  { key: "unused", label: t("share.unused"), color: "color-mix(in oklch, var(--muted-foreground) 40%, transparent)", n: usage("unused") },
                ]}
              />
            </div>
          </div>
        </div>
      </section>
      <StatStrip
        className="mb-0"
        items={[
          { label: t("strip.critical"), value: totals[0]?.n ?? 0, tone: "err" },
          { label: t("strip.high"), value: totals[1]?.n ?? 0, tone: "orange" },
          { label: t("strip.fixable"), value: findings.filter((f) => f.fixedIn).length, sub: t("strip.fixableSub", { total: findings.length }) },
          { label: t("strip.images"), value: images.length, sub: images[0] ? formatRelative(images[0].scannedAt) : undefined },
        ]}
      />
      <div className="grid grid-cols-1 gap-10 lg:grid-cols-2">
        <section>
          <h3 className="mb-3 text-sm font-medium">{t("top")}</h3>
          <ol>
            {top.map((f, i) => (
              <li key={`${f.hostId}:${f.imageId ?? f.image}:${f.id}:${f.pkg}:${f.installed}`} className="border-t border-border/50 first:border-0">
                <button type="button" onClick={() => onFocus(f.id)} className="group flex w-full items-center gap-3 rounded-lg px-2 py-2.5 text-left transition-colors hover:bg-muted">
                  <span className="w-4 text-xs text-muted-foreground">{i + 1}</span>
                  <span className="min-w-0 flex-1">
                    <span className="flex items-center gap-2">
                      <SevDot color={SEVERITIES.find((s) => s.key === f.severity)?.color ?? ""} />
                      <span className="font-mono text-sm">{f.id}</span>
                    </span>
                    <span className="block truncate font-mono text-xs text-muted-foreground">
                      {f.pkg} → {f.fixedIn ?? t("noFix")} · {short(f.image)}
                    </span>
                  </span>
                  <b className="tabular">{f.cvss.toFixed(1)}</b>
                </button>
              </li>
            ))}
          </ol>
        </section>
        <section>
          <h3 className="mb-3 text-sm font-medium">{t("perImage")}</h3>
          <ul>
            {images.map((im) => (
              <li key={`${im.hostId}:${im.id ?? im.ref}`} className="border-t border-border/50 px-2 py-2.5 first:border-0">
                <div className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate font-mono text-xs">{im.ref}</span>
                  {stackName(im.stackId) && <ToneBadge tone="accent">{stackName(im.stackId)}</ToneBadge>}
                  <b className="text-sm tabular">{totalOf(im)}</b>
                </div>
                <div className="mt-1.5 flex h-1.5 gap-0.5" style={{ width: `${Math.max(6, (totalOf(im) / max) * 100)}%` }}>
                  {SEVERITIES.map((s) => (im.counts[s.key] ? <span key={s.key} className="rounded-full" style={{ flexGrow: im.counts[s.key], background: s.color }} /> : null))}
                </div>
              </li>
            ))}
          </ul>
        </section>
      </div>
    </div>
  );
}

type FindingKey = "id" | "severity" | "pkg" | "installed" | "fixed" | "image" | "host" | "cvss" | "risk" | "epss";
type ImageKey = "image" | "stack" | "host" | "usage" | Severity | "scanned";

const FINDING_COLUMNS: FindingKey[] = ["id", "severity", "pkg", "installed", "fixed", "image", "host", "cvss", "risk", "epss"];
const SEV_RANK: Record<Severity, number> = { critical: 4, high: 3, medium: 2, low: 1, unknown: 0 };
const DAY = 86_400_000;

/** Sort state of one table: a click on the active column flips it, another column starts with its natural direction. */
function useTableSort<K extends string>(initial: SortState<K>, descFirst: readonly K[]) {
  const [sort, setSort] = useState(initial);
  const onSort = (key: K) => setSort((s) => (s.key === key ? { key, dir: s.dir === "asc" ? "desc" : "asc" } : { key, dir: descFirst.includes(key) ? "desc" : "asc" }));
  return [sort, onSort] as const;
}

const byValue = <T,>(rows: T[], dir: SortState<string>["dir"], value: (r: T) => string | number) =>
  rows.toSorted((a, b) => {
    const va = value(a);
    const vb = value(b);
    const c = typeof va === "number" && typeof vb === "number" ? va - vb : COLLATOR.compare(String(va), String(vb));
    return dir === "asc" ? c : -c;
  });

interface FrameProps<T, K extends string> {
  label: string;
  columns: { id: K; label: string; short?: string }[];
  filters: ColumnFilter<T>[];
  values: FilterValues;
  setValues: (fn: (cur: FilterValues) => FilterValues) => void;
  sort: SortState<K>;
  onSort: (key: K) => void;
  summary: { shown: number; total: number; noun: string; nounOf: string };
  empty: ReactNode;
  children: ReactNode;
}

/**
 * Card frame of the security tables, matching the Stacks and Containers
 * overviews: count and reset on top, sortable headers, a recessed filter row
 * and separated row surfaces whose left edge carries --edge.
 */
function SecurityTable<T, K extends string>({ label, columns, filters, values, setValues, sort, onSort, summary, empty, children }: FrameProps<T, K>) {
  const t = useTranslations("security");
  const tf = useTranslations("common.columnFilters");
  return (
    <Card className="gap-2 p-4" data-slot="security-table">
      <ColumnFilterSummary {...summary} values={values} onReset={() => setValues(() => ({}))} className="px-1" />
      <div className="-mx-1 overflow-x-auto px-1">
        <BareTable aria-label={label} className="border-separate border-spacing-y-1">
          <TableHeader className="[&_tr]:border-0">
            <TableRow className="hover:bg-transparent">
              {columns.map((c) => (
                <SortableHead key={c.id} sortKey={c.id} sort={sort} onSort={onSort} label={t("filters.sortBy", { column: c.label })} filtered={!!values[c.id]?.trim()} filteredLabel={tf("filtered")}>
                  {c.short ? <abbr title={c.label} className="no-underline">{c.short}</abbr> : c.label}
                </SortableHead>
              ))}
            </TableRow>
            <ColumnFilterRow columns={columns} filters={filters} values={values} onChange={(id, v) => setValues((cur) => ({ ...cur, [id]: v }))} />
          </TableHeader>
          <TableBody>
            {summary.shown === 0 && (
              <TableRow className="hover:bg-transparent">
                <TableCell colSpan={columns.length} className="rounded-xl border border-dashed py-8 text-center text-muted-foreground">
                  {empty}
                </TableCell>
              </TableRow>
            )}
            {children}
          </TableBody>
        </BareTable>
      </div>
    </Card>
  );
}

const ROW = cn("transition-colors duration-(--dur-fast) [&>td]:py-1.5", ROW_SURFACE, ROW_EDGES);
const edge = (color: string) => ({ "--host": color }) as CSSProperties;
const worst = (i: ScannedImage) => SEVERITIES.find((s) => (i.counts[s.key] ?? 0) > 0)?.color ?? "var(--border)";

export function FindingsTab({ findings, hosts, highlight }: Pick<Data, "findings" | "hosts"> & { highlight: string | null }) {
  const t = useTranslations("security");
  const tf = useTranslations("common.columnFilters");
  const [values, setValues] = useState<FilterValues>({});
  const [sort, onSort] = useTableSort<FindingKey>({ key: "risk", dir: "desc" }, ["cvss", "severity", "risk", "epss"]);
  const filters = useMemo<ColumnFilter<Finding>[]>(() => {
    const images = [...new Set(findings.map((f) => short(f.image)))].sort(COLLATOR.compare);
    const hostIds = [...new Set(findings.map((f) => f.hostId))];
    return [
      { id: "id", kind: "text", text: (f) => `${f.id} ${f.title}` },
      { id: "severity", kind: "choice", options: SEVERITIES.map((s) => ({ value: s.key, label: t(`severities.${s.key}`) })), match: (f, v) => f.severity === v },
      { id: "pkg", kind: "text", text: (f) => f.pkg },
      { id: "installed", kind: "text", text: (f) => f.installed },
      { id: "fixed", kind: "choice", options: [{ value: "yes", label: t("filters.withFix") }, { value: "no", label: t("filters.withoutFix") }], match: (f, v) => (v === "yes") === !!f.fixedIn },
      { id: "image", kind: "choice", options: images.map((i) => ({ value: i, label: i })), match: (f, v) => short(f.image) === v },
      { id: "host", kind: "choice", options: hostIds.map((h) => ({ value: h, label: hosts.get(h)?.name ?? h })), match: (f, v) => f.hostId === v },
      { id: "cvss", kind: "choice", options: ["9", "7", "4"].map((v) => ({ value: v, label: t("filters.cvssMin", { value: `${v},0` }) })), match: (f, v) => f.cvss >= Number(v) },
    ];
  }, [findings, hosts, t]);
  const value = (f: Finding, key: FindingKey): string | number =>
    key === "risk" ? (f.risk ?? f.cvss) : key === "epss" ? (f.epss ?? -1) : key === "severity" ? SEV_RANK[f.severity] : key === "cvss" ? f.cvss : key === "fixed" ? (f.fixedIn ?? "") : key === "image" ? short(f.image) : key === "host" ? (hosts.get(f.hostId)?.name ?? f.hostId) : f[key];
  const rows = byValue(applyColumnFilters(findings, filters, values), sort.dir, (f) => value(f, sort.key));
  const columns = FINDING_COLUMNS.map((c) => ({ id: c, label: t(`columns.${c}`) }));
  return (
    <SecurityTable
      label={t("tabs.findings")}
      columns={columns}
      filters={filters}
      values={values}
      setValues={setValues}
      sort={sort}
      onSort={onSort}
      summary={{ shown: rows.length, total: findings.length, noun: t("filters.findings"), nounOf: t("filters.findingsOf") }}
      empty={activeFilters(values) ? tf("empty") : t("noFindings")}
    >
      {rows.map((f) => {
        const sev = SEVERITIES.find((s) => s.key === f.severity);
        return (
          <TableRow key={`${f.hostId}:${f.imageId ?? f.image}:${f.id}:${f.pkg}:${f.installed}`} data-state={highlight === f.id ? "selected" : undefined} style={edge(sev?.color ?? "var(--border)")} className={ROW}>
            <TableCell className="font-mono text-xs" title={f.title}>
              {f.id}{f.knownExploited && <ToneBadge tone="err">{t("kev")}</ToneBadge>}
            </TableCell>
            <TableCell>
              <ToneBadge tone={sev?.tone ?? "gray"}>{t(`severities.${f.severity}`)}</ToneBadge>
            </TableCell>
            <TableCell className="font-mono text-xs">{f.pkg}</TableCell>
            <TableCell className="font-mono text-xs text-muted-foreground">{f.installed}</TableCell>
            <TableCell className="font-mono text-xs">{f.fixedIn ?? t("noFix")}</TableCell>
            <TableCell className="max-w-48 truncate font-mono text-xs">{short(f.image)}</TableCell>
            <TableCell>
              <HostChip host={hosts.get(f.hostId)} />
            </TableCell>
            <TableCell className="font-semibold tabular">{f.cvss.toFixed(1)}</TableCell>
            <TableCell className="tabular" title={f.risk === undefined ? t("unknownIntel") : undefined}>{f.risk?.toFixed(1) ?? "–"}</TableCell>
            <TableCell className="tabular">{f.epss === undefined ? "–" : `${(f.epss * 100).toFixed(2)}%`}</TableCell>
          </TableRow>
        );
      })}
    </SecurityTable>
  );
}

export function ImagesTab({ images, stacks, hosts }: Pick<Data, "images" | "stacks" | "hosts">) {
  const t = useTranslations("security");
  const tf = useTranslations("common.columnFilters");
  const [values, setValues] = useState<FilterValues>({});
  const [sort, onSort] = useTableSort<ImageKey>({ key: "critical", dir: "desc" }, ["critical", "high", "medium", "low", "scanned"]);
  const [now] = useState(() => Date.now());
  const stackName = (id?: string) => stacks.find((s) => s.id === id)?.name;
  const filters = useMemo<ColumnFilter<ScannedImage>[]>(() => {
    const stackIds = [...new Set(images.map((i) => i.stackId ?? ""))];
    const hostIds = [...new Set(images.map((i) => i.hostId))];
    const name = (id?: string) => stacks.find((s) => s.id === id)?.name;
    return [
      { id: "image", kind: "text", text: (i) => i.ref },
      { id: "stack", kind: "choice", options: stackIds.map((id) => ({ value: id || "-", label: id ? (name(id) ?? id) : t("filters.noStack") })), match: (i, v) => (i.stackId ?? "-") === v },
      { id: "host", kind: "choice", options: hostIds.map((h) => ({ value: h, label: hosts.get(h)?.name ?? h })), match: (i, v) => i.hostId === v },
      { id: "usage", kind: "choice", options: (["running", "stopped", "unused"] as const).map((u) => ({ value: u, label: t(`usage.${u}`) })), match: (i, v) => i.usage === v },
      ...SEVERITIES.map(
        (s): ColumnFilter<ScannedImage> => ({ id: s.key, kind: "choice", options: [{ value: "some", label: t("filters.withFindings") }, { value: "none", label: t("filters.none") }], match: (i, v) => (v === "some") === (i.counts[s.key] ?? 0) > 0 }),
      ),
      { id: "scanned", kind: "choice", options: [{ value: "1", label: t("filters.recent24") }, { value: "7", label: t("filters.recent7") }, { value: "older", label: t("filters.older7") }], match: (i, v) => (v === "older" ? now - Date.parse(i.scannedAt) > 7 * DAY : now - Date.parse(i.scannedAt) <= Number(v) * DAY) },
    ];
  }, [images, stacks, hosts, now, t]);
  const value = (i: ScannedImage, key: ImageKey): string | number =>
    key === "image" ? i.ref : key === "stack" ? (stackName(i.stackId) ?? "") : key === "host" ? (hosts.get(i.hostId)?.name ?? i.hostId) : key === "usage" ? t(`usage.${i.usage}`) : key === "scanned" ? Date.parse(i.scannedAt) : i.counts[key] ?? 0;
  const rows = byValue(applyColumnFilters(images, filters, values), sort.dir, (i) => value(i, sort.key));
  const columns: { id: ImageKey; label: string; short?: string }[] = [
    { id: "image", label: t("columns.image") },
    { id: "stack", label: t("columns.stack") },
    { id: "host", label: t("columns.host") },
    { id: "usage", label: t("columns.usage") },
    ...SEVERITIES.map((s) => ({ id: s.key, label: t(`severities.${s.key}`), short: t(`severities.${s.key}`).slice(0, 1) })),
    { id: "scanned", label: t("columns.scanned") },
  ];
  return (
    <SecurityTable
      label={t("tabs.images")}
      columns={columns}
      filters={filters}
      values={values}
      setValues={setValues}
      sort={sort}
      onSort={onSort}
      summary={{ shown: rows.length, total: images.length, noun: t("filters.images"), nounOf: t("filters.images") }}
      empty={tf("empty")}
    >
      {rows.map((im) => {
        const stack = stacks.find((s) => s.id === im.stackId);
        return (
          <TableRow key={`${im.hostId}:${im.id ?? im.ref}`} style={edge(worst(im))} className={ROW}>
            <TableCell className="max-w-72 truncate font-mono text-xs">{im.ref}</TableCell>
            <TableCell>{stack ? <Link to={`/stacks/${stack.id}`} className="text-primary-text hover:underline">{stack.name}</Link> : "–"}</TableCell>
            <TableCell>
              <HostChip host={hosts.get(im.hostId)} />
            </TableCell>
            <TableCell className="text-xs">{t(`usage.${im.usage}`)}</TableCell>
            {SEVERITIES.map((s) => (
              <TableCell key={s.key}>
                <ToneBadge tone={im.counts[s.key] ? s.tone : "gray"} className="min-w-8 justify-center">
                  {im.unavailable ? "–" : im.counts[s.key] ?? 0}
                </ToneBadge>
              </TableCell>
            ))}
            <TableCell className="text-xs text-muted-foreground">{im.unavailable ? t(im.scannedAt ? "scanUnavailable" : "notScanned") : formatRelative(im.scannedAt)}</TableCell>
          </TableRow>
        );
      })}
    </SecurityTable>
  );
}
