import { useState } from "react";

export const JSON_PREVIEW_LIMIT = 60;

export function stringifyJson(value: unknown, fallback = "[unserializable]"): string {
  try {
    const text = JSON.stringify(value, null, 2);
    return text === undefined ? fallback : text;
  } catch {
    return fallback;
  }
}

// Json renders a compact collapsible JSON block for payloads and definitions.
export function Json({ value, label }: { value: unknown; label?: string }) {
  const [open, setOpen] = useState(false);
  const text = stringifyJson(value);
  let oneLine = text;
  try {
    const compact = JSON.stringify(value);
    if (compact !== undefined) oneLine = compact;
  } catch {
    oneLine = text;
  }
  return (
    <div className="max-w-md">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="btn btn-ghost btn-xs font-mono text-left w-full justify-start"
        title={oneLine}
      >
        <span className="truncate">{label ? `${label} ` : ""}{oneLine.length > JSON_PREVIEW_LIMIT ? oneLine.slice(0, JSON_PREVIEW_LIMIT) + "…" : oneLine}</span>
      </button>
      {open && (
        <pre className="mt-1 max-h-64 overflow-auto rounded-box bg-base-300 p-3 text-xs leading-relaxed">
          {text}
        </pre>
      )}
    </div>
  );
}
