import { useEffect, useMemo, useRef, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useShell } from "../lib/shell";
import { findById } from "../lib/dashboard";

const SEARCHED = ["apps", "schedules", "deliveries", "workflows"] as const;

type Item = { label: string; hint: string; run: () => void };

export function CommandPalette({ nav }: { nav: { key: string; label: string }[] }) {
  const { send } = useAdmin();
  const { go } = useShell();
  const ref = useRef<HTMLDialogElement>(null);
  const [query, setQuery] = useState("");
  const [active, setActive] = useState(0);
  const [lists, setLists] = useState<Record<string, { id?: unknown }[]>>({});

  function open() {
    setQuery("");
    setActive(0);
    ref.current?.showModal();
    void Promise.all(SEARCHED.map((name) => send({ op: "dashboard.list", name }))).then((replies) => {
      const next: Record<string, { id?: unknown }[]> = {};
      replies.forEach((r, i) => {
        if (r.ok) next[SEARCHED[i]] = (r.data as { id?: unknown }[]) ?? [];
      });
      setLists(next);
    });
  }

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        if (ref.current?.open) ref.current.close();
        else open();
      }
    };
    addEventListener("keydown", onKey);
    return () => removeEventListener("keydown", onKey);
  });

  const items = useMemo<Item[]>(() => {
    const q = query.trim().toLowerCase();
    const out: Item[] = [];
    const hit = findById(lists, query);
    if (hit) out.push({ label: `Open ${hit.id}`, hint: hit.view, run: () => go(hit.view, hit.id) });
    for (const n of nav) {
      if (!q || n.label.toLowerCase().includes(q)) out.push({ label: n.label, hint: "view", run: () => go(n.key) });
    }
    return out;
  }, [query, lists, nav, go]);

  function choose(i: number) {
    items[i]?.run();
    ref.current?.close();
  }

  return (
    <>
      <button type="button" className="btn btn-ghost btn-sm w-full justify-between font-normal text-base-content/60" onClick={open}>
        Search or paste ID
        <kbd className="kbd kbd-xs">⌘K</kbd>
      </button>
      <dialog ref={ref} className="modal modal-top" aria-label="Command palette">
        <div className="modal-box mx-auto mt-24 max-w-lg p-0">
          <input
            autoFocus
            className="input input-ghost w-full border-0 border-b border-base-300 rounded-none font-mono focus:outline-none"
            placeholder="Go to view, or paste an application / schedule / delivery / workflow ID"
            value={query}
            onChange={(e) => { setQuery(e.target.value); setActive(0); }}
            onKeyDown={(e) => {
              if (e.key === "ArrowDown") { e.preventDefault(); setActive((a) => Math.min(a + 1, items.length - 1)); }
              if (e.key === "ArrowUp") { e.preventDefault(); setActive((a) => Math.max(a - 1, 0)); }
              if (e.key === "Enter") { e.preventDefault(); choose(active); }
            }}
          />
          <ul className="menu w-full p-2" role="listbox">
            {items.length === 0 && <li className="px-3 py-2 text-sm text-base-content/50">No match in the loaded rows.</li>}
            {items.map((it, i) => (
              <li key={it.label + it.hint}>
                <button type="button" className={i === active ? "menu-active" : ""} onMouseEnter={() => setActive(i)} onClick={() => choose(i)}>
                  <span className="flex-1 truncate">{it.label}</span>
                  <span className="text-xs text-base-content/50">{it.hint}</span>
                </button>
              </li>
            ))}
          </ul>
        </div>
        <form method="dialog" className="modal-backdrop"><button aria-label="Close">close</button></form>
      </dialog>
    </>
  );
}
