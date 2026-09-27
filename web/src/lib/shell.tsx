import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { useAdmin } from "./admin";
import { hashFor, parseHash } from "./dashboard";

export type App = { id: string; namespace: string; created_at: string; connected: boolean };

type Toast = { id: number; text: string; ok: boolean };
type ConfirmRequest = { title: string; body: string; action: string; resolve: (yes: boolean) => void };

type Ctx = {
  view: string;
  selected?: string;
  go: (view: string, id?: string) => void;
  select: (id?: string) => void;
  refresh: number;
  bump: () => void;
  updatedAt: number | null;
  markUpdated: () => void;
  notify: (text: string, ok?: boolean) => void;
  confirm: (req: Omit<ConfirmRequest, "resolve">) => Promise<boolean>;
  apps: App[];
  appName: (id: string) => string;
};

const ShellCtx = createContext<Ctx | null>(null);

export function useShell(): Ctx {
  const c = useContext(ShellCtx);
  if (!c) throw new Error("useShell outside provider");
  return c;
}

const POLL_MS = 5000;

export function ShellProvider({ defaultView, children }: { defaultView: string; children: ReactNode }) {
  const { send, state } = useAdmin();
  const [route, setRoute] = useState(() => parseHash(location.hash));
  const [refresh, setRefresh] = useState(0);
  const [updatedAt, setUpdatedAt] = useState<number | null>(null);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [pending, setPending] = useState<ConfirmRequest | null>(null);
  const [apps, setApps] = useState<App[]>([]);
  const toastId = useRef(0);
  const dialogRef = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const onHash = () => setRoute(parseHash(location.hash));
    addEventListener("hashchange", onHash);
    return () => removeEventListener("hashchange", onHash);
  }, []);

  const go = useCallback((view: string, id?: string) => {
    location.hash = hashFor(view, id);
  }, []);
  const view = route.view || defaultView;
  const select = useCallback((id?: string) => go(view, id), [go, view]);

  const bump = useCallback(() => setRefresh((r) => r + 1), []);
  const markUpdated = useCallback(() => setUpdatedAt(Date.now()), []);

  // ponytail: polling, not push. One shared tick for the visible view only;
  // upgrade to a server broadcast when query-plan checks show polling cost.
  useEffect(() => {
    if (state !== "open") return;
    const t = setInterval(() => {
      if (document.visibilityState === "visible") bump();
    }, POLL_MS);
    return () => clearInterval(t);
  }, [state, bump]);

  useEffect(() => {
    if (state !== "open") return;
    void send({ op: "dashboard.list", name: "apps" }).then((r) => {
      if (r.ok) setApps((r.data as App[]) ?? []);
    });
  }, [send, state, refresh]);

  const names = useMemo(() => new Map(apps.map((a) => [a.id, a.namespace])), [apps]);
  const appName = useCallback((id: string) => names.get(id) ?? `${id.slice(0, 8)}…`, [names]);

  const notify = useCallback((text: string, ok = true) => {
    const id = ++toastId.current;
    setToasts((t) => [...t, { id, text, ok }]);
    setTimeout(() => setToasts((t) => t.filter((x) => x.id !== id)), 4000);
  }, []);

  const confirm = useCallback(
    (req: Omit<ConfirmRequest, "resolve">) => new Promise<boolean>((resolve) => setPending({ ...req, resolve })),
    [],
  );

  useEffect(() => {
    if (pending) dialogRef.current?.showModal();
  }, [pending]);

  function answer(yes: boolean) {
    pending?.resolve(yes);
    setPending(null);
    dialogRef.current?.close();
  }

  const value = useMemo<Ctx>(
    () => ({ view, selected: route.id, go, select, refresh, bump, updatedAt, markUpdated, notify, confirm, apps, appName }),
    [view, route.id, go, select, refresh, bump, updatedAt, markUpdated, notify, confirm, apps, appName],
  );

  return (
    <ShellCtx.Provider value={value}>
      {children}
      <dialog ref={dialogRef} className="modal" onCancel={(e) => { e.preventDefault(); answer(false); }}>
        {pending && (
          <div className="modal-box max-w-md">
            <h3 className="text-lg font-semibold">{pending.title}</h3>
            <p className="py-3 text-sm text-base-content/70">{pending.body}</p>
            <div className="modal-action">
              <button className="btn btn-ghost" autoFocus onClick={() => answer(false)}>Keep</button>
              <button className="btn btn-error" onClick={() => answer(true)}>{pending.action}</button>
            </div>
          </div>
        )}
      </dialog>
      <div className="toast toast-end z-50" aria-live="polite">
        {toasts.map((t) => (
          <div key={t.id} role="status" className={`alert ${t.ok ? "alert-success" : "alert-error"} py-2 text-sm shadow-lg`}>
            {t.text}
          </div>
        ))}
      </div>
    </ShellCtx.Provider>
  );
}
