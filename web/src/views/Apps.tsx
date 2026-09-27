import { useMemo, useState } from "react";
import { useAdmin } from "../lib/admin";
import { useList } from "../lib/useList";
import { useShell, type App } from "../lib/shell";
import { DataTable, type Column } from "../components/DataTable";
import { AppName, Drawer, Fields, IdChip, Section } from "../components/ui";
import { fmt } from "../lib/format";

type Token = { id: string; application_id: string; created_at: string; revoked_at: string | null };

function Secret({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div>
      <span className="text-xs uppercase tracking-wider text-base-content/50">{label}</span>
      <div className="flex items-start gap-2">
        <pre className="flex-1 break-all whitespace-pre-wrap rounded-box bg-base-300 p-3 text-xs">{value}</pre>
        <button
          className="btn btn-sm"
          onClick={async () => {
            await navigator.clipboard.writeText(value);
            setCopied(true);
          }}
        >
          {copied ? "Copied ✓" : "Copy"}
        </button>
      </div>
    </div>
  );
}

export function Apps() {
  const { send } = useAdmin();
  const { apps, selected, select, bump, notify, confirm } = useShell();
  const { rows: tokens, reload: reloadTokens } = useList<Token>("tokens");
  const [busy, setBusy] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("");
  const [created, setCreated] = useState<{ applicationId: string; token: string } | null>(null);
  const [rotated, setRotated] = useState<{ tokenId: string; token: string } | null>(null);

  const active = useMemo(() => {
    const c = new Map<string, number>();
    for (const t of tokens) if (!t.revoked_at) c.set(t.application_id, (c.get(t.application_id) ?? 0) + 1);
    return c;
  }, [tokens]);
  const current = apps.find((a) => a.id === selected);
  const appTokens = tokens.filter((t) => t.application_id === selected);

  async function create() {
    setBusy(true);
    try {
      const r = await send({ op: "app.create", name, namespace });
      if (!r.ok) {
        notify(r.error ?? "app.create failed", false);
        return;
      }
      setCreated(r.data as { applicationId: string; token: string });
      setShowCreate(false);
      setName("");
      setNamespace("");
      bump();
    } finally {
      setBusy(false);
    }
  }

  async function rotate(app: App) {
    setBusy(true);
    try {
      const r = await send({ op: "app.token.rotate", applicationId: app.id });
      if (!r.ok) {
        notify(r.error ?? "rotation failed", false);
        return;
      }
      setRotated(r.data as { tokenId: string; token: string });
      void reloadTokens();
    } finally {
      setBusy(false);
    }
  }

  async function revoke(app: App, t: Token) {
    const yes = await confirm({
      title: `Revoke token ${t.id.slice(0, 10)}…?`,
      body: `Any ${app.namespace} process still using this token is disconnected at its next authentication. This cannot be undone.`,
      action: "Revoke token",
    });
    if (!yes) return;
    const r = await send({ op: "app.token.revoke", tokenId: t.id });
    const done = r.ok && (r.data as { revoked?: boolean })?.revoked;
    notify(done ? "Token revoked" : r.error ?? "Token was already revoked", !!done);
    void reloadTokens();
  }

  const columns: Column<App>[] = [
    { key: "namespace", label: "Application", render: (a) => <AppName id={a.id} /> },
    { key: "id", label: "Application ID", render: (a) => <IdChip value={a.id} /> },
    { key: "tokens", label: "Active tokens", render: (a) => <span className="tabular-nums">{active.get(a.id) ?? 0}</span> },
    { key: "created", label: "Created", render: (a) => <span className="text-xs text-base-content/60">{fmt(a.created_at)}</span> },
  ];

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Applications</h2>
          <p className="text-sm text-base-content/60">Registered clients, their connection state, and scoped tokens.</p>
        </div>
        <button className="btn btn-primary btn-sm" onClick={() => setShowCreate(true)}>+ Register application</button>
      </header>

      <DataTable
        columns={columns}
        rows={apps}
        empty="No applications registered."
        onRowClick={(a) => { setRotated(null); select(a.id); }}
        selectedId={selected}
      />

      {current && (
        <Drawer title={current.namespace} onClose={() => { setRotated(null); select(); }}>
          <Fields
            rows={[
              ["Application ID", <IdChip value={current.id} />],
              ["Connection", current.connected ? "connected" : "offline"],
              ["Created", fmt(current.created_at)],
            ]}
          />
          <Section title="Tokens">
            <ul className="divide-y divide-base-300 rounded-box border border-base-300">
              {appTokens.map((t) => (
                <li key={t.id} className="flex items-center justify-between gap-2 px-3 py-2 text-sm">
                  <div className="min-w-0">
                    <IdChip value={t.id} />
                    <div className="text-xs text-base-content/50">
                      created {fmt(t.created_at)}
                      {t.revoked_at && ` · revoked ${fmt(t.revoked_at)}`}
                      {t.id === rotated?.tokenId && " · new"}
                    </div>
                  </div>
                  {t.revoked_at ? (
                    <span className="badge badge-ghost badge-sm">revoked</span>
                  ) : (
                    <button className="btn btn-ghost btn-xs text-error" onClick={() => void revoke(current, t)}>Revoke</button>
                  )}
                </li>
              ))}
            </ul>
          </Section>
          {rotated ? (
            <div className="space-y-2">
              <p className="text-sm text-warning">Copy the new token now; it is stored as a hash and cannot be shown again.</p>
              <Secret label="New token" value={rotated.token} />
              <p className="text-xs text-base-content/60">Once every instance uses it, revoke the older tokens above.</p>
            </div>
          ) : (
            <div className="space-y-1">
              <button className="btn btn-sm" disabled={busy} onClick={() => void rotate(current)}>
                {busy && <span className="loading loading-spinner loading-sm" />}Rotate token
              </button>
              <p className="text-xs text-base-content/50">Adds a token. Old tokens keep working until you revoke them.</p>
            </div>
          )}
        </Drawer>
      )}

      {showCreate && (
        <dialog className="modal modal-open">
          <div className="modal-box">
            <h3 className="text-lg font-semibold">Register application</h3>
            <p className="py-2 text-sm text-base-content/60">Namespace must be unique. The token is shown once.</p>
            <label className="form-control w-full">
              <span className="label-text mb-1">Name</span>
              <input className="input input-bordered" value={name} onChange={(e) => setName(e.target.value)} autoFocus />
            </label>
            <label className="form-control mt-3 w-full">
              <span className="label-text mb-1">Namespace</span>
              <input className="input input-bordered font-mono" value={namespace} onChange={(e) => setNamespace(e.target.value)} />
            </label>
            <div className="modal-action">
              <button className="btn btn-ghost" onClick={() => setShowCreate(false)}>Cancel</button>
              <button className="btn btn-primary" disabled={busy || !name || !namespace} onClick={() => void create()}>
                {busy && <span className="loading loading-spinner loading-sm" />}Create
              </button>
            </div>
          </div>
          <div className="modal-backdrop" onClick={() => setShowCreate(false)} />
        </dialog>
      )}

      {created && (
        <dialog className="modal modal-open">
          <div className="modal-box space-y-3">
            <h3 className="text-lg font-semibold">Application created</h3>
            <p className="text-sm text-warning">Copy the token now; it is stored as a hash and cannot be shown again.</p>
            <Secret label="Application ID" value={created.applicationId} />
            <Secret label="Token" value={created.token} />
            <div className="modal-action">
              <button className="btn btn-primary" onClick={() => setCreated(null)}>I saved the token</button>
            </div>
          </div>
        </dialog>
      )}
    </div>
  );
}
