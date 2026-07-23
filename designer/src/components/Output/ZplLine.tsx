import { tokenizeZplLine, type ZplTokenType } from "../../lib/zplTokenize";

/** Tailwind colour per ZPL token type (see index.css theme tokens). */
const TOKEN_CLASS: Record<ZplTokenType, string> = {
  structural: "text-accent font-semibold",
  command: "text-accent font-medium",
  fieldData: "text-string",
  comment: "text-muted italic",
  number: "text-info",
  enum: "text-text",
  separator: "text-muted",
  text: "text-muted",
};

// A ~DY/~DG payload (embedded font or graphic hex) can be megabytes on a
// single line. Tokenizing it splits the hex into millions of tokens, one
// <span> each, which freezes the pane. Highlight only the head and summarise
// the remainder; copy uses the full generated string, so nothing is lost.
const MAX_LINE_RENDER = 2000;

/** One line of rendered ZPL, syntax-highlighted per token. Shared between
 *  the per-label ZPL pane and the Setup-Script preview pane. */
export function ZplLine({ line }: { line: string }) {
  const truncated = line.length > MAX_LINE_RENDER;
  const tokens = tokenizeZplLine(truncated ? line.slice(0, MAX_LINE_RENDER) : line);
  return (
    <span className="block">
      {/* A blank line collapses to zero height inside <pre>; keep its row. */}
      {tokens.length === 0
        ? "\n"
        : tokens.map((tok, i) => (
            <span key={i} className={TOKEN_CLASS[tok.type]}>
              {tok.value}
            </span>
          ))}
      {truncated && (
        <span className="text-muted italic">
          {` …(+${(line.length - MAX_LINE_RENDER).toLocaleString()})`}
        </span>
      )}
    </span>
  );
}
