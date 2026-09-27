import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { AdminClient, AuthRejected, wsURL, type AdminCommand, type CommandResult, type ConnState, type Reply } from "../lib/protocol";

type Ctx = {
  state: ConnState;
  // session is true from the first successful connect until disconnect, so a
  // dropped socket keeps the shell visible while it reconnects.
  session: boolean;
  connect: (token: string) => Promise<void>;
  disconnect: () => void;
  send: <C extends AdminCommand>(body: C) => Promise<Reply<CommandResult<C>>>;
  error: string | null;
};

const AdminCtx = createContext<Ctx | null>(null);

export function useAdmin(): Ctx {
  const c = useContext(AdminCtx);
  if (!c) throw new Error("useAdmin outside provider");
  return c;
}

export function AdminProvider({ children }: { children: React.ReactNode }) {
  const clientRef = useRef<AdminClient | null>(null);
  // Held in memory only so a dropped socket can re-authenticate; never persisted.
  const tokenRef = useRef<string | null>(null);
  const [state, setState] = useState<ConnState>("idle");
  const [session, setSession] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => () => clientRef.current?.close(), []);

  // connect and send are stable: they read the live client through the ref, so
  // their identity never changes with connection state. Views can depend on
  // `send` without re-firing effects on every reconnect or error.
  const connect = useCallback(async (token: string) => {
    clientRef.current?.close();
    const c = new AdminClient(wsURL(), token);
    c.setHandlers({ onState: setState });
    clientRef.current = c;
    try {
      await c.connect();
      tokenRef.current = token;
      setSession(true);
      setError(null);
    } catch (e) {
      c.close();
      if (clientRef.current === c) clientRef.current = null;
      if (e instanceof AuthRejected) {
        tokenRef.current = null;
        setSession(false);
      }
      setState("closed");
      setError(e instanceof Error ? e.message : "Calcron connection failed");
    }
  }, []);

  const disconnect = useCallback(() => {
    tokenRef.current = null;
    clientRef.current?.close();
    clientRef.current = null;
    setSession(false);
    setState("idle");
  }, []);

  useEffect(() => {
    if (!session || state !== "closed" || !tokenRef.current) return;
    const token = tokenRef.current;
    const t = setTimeout(() => void connect(token), 2000);
    return () => clearTimeout(t);
  }, [session, state, connect]);

  const send = useCallback(async <C extends AdminCommand>(body: C): Promise<Reply<CommandResult<C>>> => {
    const c = clientRef.current;
    if (!c) return { ok: false, error: "not connected" };
    try {
      const r = await c.request(body);
      setError(r.ok ? null : (r.error ?? "request failed"));
      return r as Reply<CommandResult<C>>;
    } catch (e) {
      const error = e instanceof Error ? e.message : "request failed";
      setError(error);
      return { ok: false, error };
    }
  }, []);

  const value = useMemo<Ctx>(
    () => ({ state, session, connect, disconnect, send, error }),
    [state, session, connect, disconnect, send, error],
  );

  return <AdminCtx.Provider value={value}>{children}</AdminCtx.Provider>;
}
