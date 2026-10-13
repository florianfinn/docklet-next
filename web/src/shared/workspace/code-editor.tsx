import { defaultKeymap, history, historyKeymap, indentWithTab } from "@codemirror/commands";
import { bracketMatching, HighlightStyle, indentOnInput, syntaxHighlighting } from "@codemirror/language";
import { type Diagnostic, forceLinting, linter, lintGutter } from "@codemirror/lint";
import { EditorState } from "@codemirror/state";
import { drawSelection, EditorView, highlightActiveLine, highlightActiveLineGutter, keymap, lineNumbers, placeholder } from "@codemirror/view";
import { tags } from "@lezer/highlight";
import { useEffect, useRef } from "react";

import { languageExtension, type EditorLanguage } from "./languages";

export interface EditorDiagnostic {
  line: number;
  message: string;
  severity: "error" | "warning";
}

/* Colours follow Arcane's --syntax-* tokens so light and dark themes match. */
const highlight = HighlightStyle.define([
  { tag: tags.comment, color: "var(--syntax-comment)", fontStyle: "italic" },
  { tag: [tags.propertyName, tags.definition(tags.propertyName)], color: "var(--syntax-property)" },
  { tag: [tags.string, tags.special(tags.string)], color: "var(--syntax-string)" },
  { tag: [tags.number, tags.bool, tags.null, tags.variableName], color: "var(--syntax-number)" },
  { tag: [tags.keyword, tags.operator, tags.punctuation, tags.separator], color: "var(--syntax-keyword)" },
  { tag: tags.heading, color: "var(--syntax-keyword)", fontWeight: "600" },
]);

const theme = EditorView.theme({
  "&": { height: "100%", backgroundColor: "transparent", color: "var(--foreground)", fontSize: "12.5px" },
  ".cm-scroller": { fontFamily: "var(--font-mono)", lineHeight: "1.65" },
  ".cm-content": { padding: "12px 0", caretColor: "var(--primary)" },
  ".cm-gutters": { backgroundColor: "transparent", color: "var(--muted-foreground)", borderRight: "1px solid var(--border)" },
  ".cm-activeLine": { backgroundColor: "color-mix(in oklch, var(--muted) 45%, transparent)" },
  ".cm-activeLineGutter": { backgroundColor: "color-mix(in oklch, var(--muted) 60%, transparent)", color: "var(--foreground)" },
  "&.cm-focused": { outline: "none" },
  "&.cm-focused .cm-cursor": { borderLeftColor: "var(--primary)" },
  ".cm-selectionBackground, &.cm-focused .cm-selectionBackground, ::selection": { backgroundColor: "color-mix(in oklch, var(--primary) 30%, transparent) !important" },
  ".cm-lintRange-error": { backgroundImage: "none", textDecoration: "underline wavy var(--destructive)" },
  ".cm-lintRange-warning": { backgroundImage: "none", textDecoration: "underline wavy var(--warning)" },
  ".cm-tooltip": { backgroundColor: "var(--popover)", border: "1px solid var(--border)", borderRadius: "10px" },
  ".cm-diagnostic-error": { borderLeftColor: "var(--destructive)" },
  ".cm-diagnostic-warning": { borderLeftColor: "var(--warning)" },
});

interface CodeEditorProps {
  value: string;
  onChange?: (value: string) => void;
  language: EditorLanguage;
  diagnostics?: EditorDiagnostic[];
  label: string;
  describedBy?: string;
  placeholderText?: string;
  readOnly?: boolean;
  /** Bound to Ctrl/Cmd+S inside the editor. */
  onSave?: () => void;
}

const NO_DIAGNOSTICS: EditorDiagnostic[] = [];

/** CodeMirror 6 editor shared by every workspace; external value changes replace the document. */
export function CodeEditor({ value, onChange, language, diagnostics = NO_DIAGNOSTICS, label, describedBy, placeholderText, readOnly = false, onSave }: CodeEditorProps) {
  const host = useRef<HTMLDivElement>(null);
  const view = useRef<EditorView | null>(null);
  const onChangeRef = useRef(onChange);
  const onSaveRef = useRef(onSave);
  const diagRef = useRef(diagnostics);

  useEffect(() => {
    onChangeRef.current = onChange;
    onSaveRef.current = onSave;
  });

  useEffect(() => {
    diagRef.current = diagnostics;
    if (view.current) forceLinting(view.current);
  }, [diagnostics]);

  useEffect(() => {
    if (!host.current) return;
    const lint = linter(
      (v) =>
        diagRef.current.flatMap((d): Diagnostic[] => {
          if (d.line < 1 || d.line > v.state.doc.lines) return [];
          const line = v.state.doc.line(d.line);
          return [{ from: line.from, to: Math.max(line.from, line.to), severity: d.severity, message: d.message }];
        }),
      { delay: 150 },
    );
    const save = { key: "Mod-s", preventDefault: true, run: () => (onSaveRef.current ? (onSaveRef.current(), true) : false) };
    const state = EditorState.create({
      doc: value,
      extensions: [
        lineNumbers(),
        highlightActiveLineGutter(),
        history(),
        drawSelection(),
        indentOnInput(),
        bracketMatching(),
        highlightActiveLine(),
        lintGutter(),
        lint,
        keymap.of([save, ...defaultKeymap, ...historyKeymap, indentWithTab]),
        languageExtension(language),
        syntaxHighlighting(highlight),
        EditorState.tabSize.of(2),
        EditorState.readOnly.of(readOnly),
        theme,
        placeholder(placeholderText ?? ""),
        EditorView.contentAttributes.of({ "aria-label": label, ...(describedBy ? { "aria-describedby": describedBy } : {}), ...(readOnly ? { "aria-readonly": "true" } : {}) }),
        EditorView.updateListener.of((u) => {
          if (u.docChanged) onChangeRef.current?.(u.state.doc.toString());
        }),
      ],
    });
    view.current = new EditorView({ state, parent: host.current });
    return () => {
      view.current?.destroy();
      view.current = null;
    };
    // Recreated per file, language and mode; value updates are applied below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [language, label, readOnly]);

  useEffect(() => {
    const v = view.current;
    if (!v) return;
    const current = v.state.doc.toString();
    if (current !== value) v.dispatch({ changes: { from: 0, to: current.length, insert: value } });
  }, [value]);

  return <div ref={host} data-slot="code-editor" data-language={language} className="h-full min-h-0 overflow-hidden" />;
}
