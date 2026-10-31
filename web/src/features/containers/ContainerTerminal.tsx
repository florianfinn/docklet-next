// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original lazy xterm terminal with owned fit, input and reconnect lifetimes.

import { FitAddon } from "@xterm/addon-fit";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { RotateCw } from "lucide-react";
import { useEffect, useId, useRef, useState } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import type { ShellEvent, ShellOptions } from "@/data/shell";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader } from "@/shared/ui/card";
import { Field, FieldLabel } from "@/shared/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { shellMessage } from "./shell-message";

export default function ContainerTerminal({ containerId }: { containerId: string }) {
  const adapter = useAdapter();
  const t = useTranslations("containers.terminal");
  const node = useRef<HTMLDivElement>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const selectId = useId();
  const [shell, setShell] = useState<ShellOptions["shell"]>("auto");
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<Exclude<ShellEvent, { type: "output" }>>({ type: "connecting" });
  const inputLabel = t("input");
  const labelRef = useRef(inputLabel);
  useEffect(() => {
    const element = node.current;
    if (!element) return;
    let active = true, ended = false;
    const style = getComputedStyle(element);
    const terminal = new Terminal({ scrollback: 2000, fontSize: 13, fontFamily: style.fontFamily || "monospace", screenReaderMode: true, disableStdin: true, theme: { background: style.backgroundColor || undefined, foreground: style.color || undefined } });
    const fit = new FitAddon();
    terminal.loadAddon(fit); terminal.open(element); terminalRef.current = terminal;
    terminal.textarea?.setAttribute("aria-label", labelRef.current);
    const size = () => {
      if (!active) return;
      if (element.clientWidth > 0 && element.clientHeight > 0) fit.fit();
      session?.resize(Math.min(1000, Math.max(1, terminal.cols)), Math.min(1000, Math.max(1, terminal.rows)));
    };
    if (element.clientWidth > 0 && element.clientHeight > 0) fit.fit();
    const session = adapter.openContainerShell(containerId, { shell, cols: Math.min(1000, terminal.cols), rows: Math.min(1000, terminal.rows) }, (event) => {
      if (!active || ended) return;
      if (event.type === "output") { terminal.write(event.data); return; }
      if (event.type === "exit") ended = true;
      terminal.options.disableStdin = event.type !== "live";
      setState(event);
      if (event.type === "live") terminal.focus();
    });
    const input = terminal.onData((data) => session?.send(data));
    const resize = terminal.onResize(({ cols, rows }) => session?.resize(Math.min(1000, cols), Math.min(1000, rows)));
    const observer = new ResizeObserver(size); observer.observe(element);
    return () => { active = false; observer.disconnect(); input.dispose(); resize.dispose(); session?.close(); terminal.dispose(); terminalRef.current = null; };
  }, [adapter, containerId, shell, attempt]);
  useEffect(() => { labelRef.current = inputLabel; terminalRef.current?.textarea?.setAttribute("aria-label", inputLabel); }, [inputLabel]);
  const status = state.type === "exit" ? state.reason ? t(shellMessage(state.reason)) : t("ended", { code: state.code === null ? t("unknownCode") : String(state.code) }) : t(state.type);
  return (
    <Card className="gap-0 overflow-hidden py-0">
      <CardHeader className="flex flex-row flex-wrap items-center gap-3 border-b px-4 py-3">
        <Field orientation="horizontal" className="w-auto">
          <FieldLabel htmlFor={selectId}>{t("shell")}</FieldLabel>
          <Select value={shell} onValueChange={(value) => { if (value === "auto" || value === "sh" || value === "bash") setShell(value); }}>
            <SelectTrigger id={selectId} aria-label={t("shell")}><SelectValue /></SelectTrigger>
            <SelectContent><SelectGroup>{(["auto", "sh", "bash"] as const).map((choice) => <SelectItem key={choice} value={choice}>{t(choice)}</SelectItem>)}</SelectGroup></SelectContent>
          </Select>
        </Field>
        <span className="flex-1" />
        <Button size="sm" variant="outline" onClick={() => setAttempt((value) => value + 1)}><RotateCw data-icon="inline-start" />{t("reconnect")}</Button>
      </CardHeader>
      <CardContent className="px-3 py-3">
        <p role="status" className="mb-3 text-sm text-muted-foreground">{status}</p>
        <div ref={node} className="h-80 min-w-0 overflow-hidden bg-code font-mono text-foreground" />
      </CardContent>
    </Card>
  );
}
