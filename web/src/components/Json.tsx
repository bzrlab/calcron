import { useState } from "react";

// Json renders a compact collapsible JSON block for payloads and definitions.
export function Json({ value, label }: { value: unknown; label?: string }) {
  const [open, setOpen] = useState(false);
  const text = JSON.stringify(value ?? {}, null, 2);
  const oneLine = JSON.stringify(value ?? {});
  return (
    <div className="max-w-md">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        className="btn btn-ghost btn-xs font-mono text-left w-full justify-start"
        title={oneLine}
      >
        <span className="truncate">{label ? `${label} ` : ""}{oneLine.length > 60 ? oneLine.slice(0, 60) + "…" : oneLine}</span>
      </button>
      {open && (
        <pre className="mt-1 max-h-64 overflow-auto rounded-box bg-base-300 p-3 text-xs leading-relaxed">
          {text}
        </pre>
      )}
    </div>
  );
}
