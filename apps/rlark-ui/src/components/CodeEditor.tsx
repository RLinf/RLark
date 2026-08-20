import { useMemo, useState, type ChangeEventHandler } from "react";

function LineNumbers({
  count,
  offset = 0,
}: {
  count: number;
  offset?: number;
}) {
  return (
    <div
      className="code-editor-lines"
      aria-hidden="true"
      style={{ transform: `translateY(${-offset}px)` }}
    >
      {Array.from({ length: Math.max(1, count) }, (_, index) => (
        <span key={index}>{index + 1}</span>
      ))}
    </div>
  );
}

const highlightTokens = new Set([
  "python",
  "python3",
  "bash",
  "sh",
  "node",
  "npm",
  "pnpm",
  "yarn",
  "torchrun",
  "kubectl",
  "rlark",
  "pip",
  "pip3",
]);

function HighlightedCode({ code }: { code: string }) {
  const lines = code.split("\n");
  return (
    <>
      {lines.map((line, lineIndex) => (
        <span
          className="code-editor-highlight-line"
          key={`${lineIndex}-${line}`}
        >
          {highlightCodeLine(line || " ")}
          {lineIndex < lines.length - 1 ? "\n" : null}
        </span>
      ))}
    </>
  );
}

function highlightCodeLine(line: string) {
  return line.split(/(\s+|&&|\|\||[|;])/).map((part, index) => {
    if (!part) return null;
    const className =
      highlightTokens.has(part) || part.startsWith("-m")
        ? "code-token-keyword"
        : part.startsWith("--") || part.startsWith("-")
          ? "code-token-flag"
          : part.startsWith("/") || part.includes("=")
            ? "code-token-value"
            : "";
    return className ? (
      <span className={className} key={`${part}-${index}`}>
        {part}
      </span>
    ) : (
      <span key={`${part}-${index}`}>{part}</span>
    );
  });
}

export function CodeEditorField({
  value,
  onChange,
  placeholder,
  minHeight = 112,
  label = "script.sh",
  language = "Shell",
}: {
  value: string;
  onChange: ChangeEventHandler<HTMLTextAreaElement>;
  placeholder?: string;
  minHeight?: number;
  label?: string;
  language?: string;
}) {
  const [scrollTop, setScrollTop] = useState(0);
  const lineCount = useMemo(() => value.split("\n").length, [value]);
  return (
    <div
      className="code-editor-shell code-editor-input"
      aria-label={`${label} ${language}`}
      data-language={language}
    >
      <div className="code-editor-body" style={{ minHeight }}>
        <LineNumbers count={lineCount} offset={scrollTop} />
        <textarea
          value={value}
          onChange={onChange}
          onScroll={(event) => {
            setScrollTop(event.currentTarget.scrollTop);
          }}
          placeholder={placeholder}
          spellCheck={false}
          autoCapitalize="off"
          autoCorrect="off"
          style={{ minHeight }}
        />
      </div>
    </div>
  );
}

export function CodeBlock({
  code,
  label = "script.sh",
  language = "Shell",
}: {
  code: string;
  label?: string;
  language?: string;
}) {
  const lines = code.split("\n");
  return (
    <div
      className="code-editor-shell code-editor-viewer"
      aria-label={`${label} ${language}`}
      data-language={language}
    >
      <div className="code-editor-body">
        <LineNumbers count={lines.length} />
        <pre>
          <code>
            <HighlightedCode code={code} />
          </code>
        </pre>
      </div>
    </div>
  );
}
