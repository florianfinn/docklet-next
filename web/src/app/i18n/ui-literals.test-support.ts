import ts from "typescript";

const TEXT_FIELDS = /^(?:aria-label|aria-description|aria-valuetext|title|alt|placeholder|label|description|caption|heading|subtitle|text|message|emptyText|helpText|tooltip|ariaLabel)$/;
const TECHNICAL_FIELDS = /^(?:className|class|style|d|points|viewBox|path|pattern|accept|src|href|id|key|name|type|role|value|variant|size|data-.*|aria-(?!label$|description$|valuetext$).*)$/;
const TECHNICAL_CALLS = /^(?:cn|clsx|cva|useTranslations|t|\w+\.t|\w+\.raw|\w+\.rich|\w+\.has|querySelector|querySelectorAll|.*\.querySelector|.*\.querySelectorAll|.*\.matches|.*\.closest)$/;

export interface UiLiteral { text: string; line: number }

/** Inspect UI sinks and prose in every implementation file, independently of language. */
export function uiLiterals(file: string, source: string): UiLiteral[] {
  const sf = ts.createSourceFile(file, source, ts.ScriptTarget.Latest, true, file.endsWith("x") ? ts.ScriptKind.TSX : ts.ScriptKind.TS);
  const found: UiLiteral[] = [];
  const visit = (node: ts.Node) => {
    if (ts.isImportDeclaration(node) || ts.isExportDeclaration(node) || ts.isTypeNode(node)) return;
    let text: string | undefined;
    if (ts.isStringLiteral(node) || ts.isNoSubstitutionTemplateLiteral(node)) text = node.text;
    else if (ts.isTemplateExpression(node)) text = [node.head.text, ...node.templateSpans.map((span) => span.literal.text)].join("{}");
    else if (ts.isJsxText(node)) text = node.text.trim();
    if (text?.trim() && /\p{L}/u.test(text)) {
      let context: ts.Node = node;
      while (context.parent) {
        const parent = context.parent;
        const output = ts.isJsxExpression(parent) || ts.isParenthesizedExpression(parent)
          || ts.isTemplateSpan(parent) || ts.isTemplateExpression(parent)
          || (ts.isConditionalExpression(parent) && parent.condition !== context)
          || (ts.isBinaryExpression(parent) && ([ts.SyntaxKind.BarBarToken, ts.SyntaxKind.QuestionQuestionToken].includes(parent.operatorToken.kind)
            || (parent.operatorToken.kind === ts.SyntaxKind.AmpersandAmpersandToken && parent.right === context)));
        if (!output) break;
        context = parent;
      }
      const parent = context.parent;
      const field = parent && (ts.isJsxAttribute(parent) || ts.isPropertyAssignment(parent)) ? parent.name.getText(sf) : "";
      const isKey = parent && ts.isPropertyAssignment(parent) && parent.name === context;
      const technicalCall = parent && ts.isCallExpression(parent) && TECHNICAL_CALLS.test(parent.expression.getText(sf));
      const technical = TECHNICAL_FIELDS.test(field) || technicalCall || isKey;
      const jsxChild = ts.isJsxText(node) || (ts.isJsxExpression(context) && parent && (ts.isJsxElement(parent) || ts.isJsxFragment(parent)));
      const prose = /(?:^|[\s.,;:!?])\p{L}[\p{L}\p{M}'’]*\s+[\p{L}\p{M}]+(?:$|[\s.,;:!?])/u.test(text);
      const directive = ts.isExpressionStatement(node.parent) && /^(?:use client|use server|use strict)$/.test(text);
      const css = /^(?:color-mix|(?:linear|radial)-gradient)\(/.test(text);
      if (!technical && !directive && (!css || jsxChild || TEXT_FIELDS.test(field)) && (jsxChild || TEXT_FIELDS.test(field) || prose)) {
        found.push({ text, line: sf.getLineAndCharacterOfPosition(node.getStart(sf)).line + 1 });
      }
      if (ts.isTemplateExpression(node)) {
        node.templateSpans.forEach((span) => visit(span.expression));
        return;
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(sf);
  return found;
}
