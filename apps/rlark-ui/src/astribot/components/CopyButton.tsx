import { useState } from "react";
import { Check, Copy as CopyIcon } from "lucide-react";
import { copyText } from "../../utils/clipboard.js";

// The copy button of our own pages: a text button that says "Copied" for a
// moment, or with iconOnly an icon that turns into a check.
export function CopyButton({
  value,
  zh,
  label = zh ? "复制" : "Copy",
  className,
  iconOnly = false,
}: {
  value: string;
  zh: boolean;
  label?: string;
  className?: string;
  iconOnly?: boolean;
}) {
  const [copied, setCopied] = useState(false);
  const shown = copied ? (zh ? "已复制" : "Copied") : label;
  return (
    <button
      type="button"
      className={className ?? (iconOnly ? `icon-button${copied ? " copy-confirmed" : ""}` : "secondary-button")}
      aria-label={iconOnly ? label : undefined}
      title={iconOnly ? shown : undefined}
      onClick={(event) => {
        event.stopPropagation();
        void copyText(value).then((ok) => {
          setCopied(ok);
          if (ok) window.setTimeout(() => setCopied(false), 1500);
        });
      }}
    >
      {iconOnly ? copied ? <Check size={14} /> : <CopyIcon size={14} /> : shown}
    </button>
  );
}
