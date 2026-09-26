import { useCallback, useEffect, useState } from "react";
import { useAdmin } from "../lib/admin";
import { DataTable, type Column } from "../components/DataTable";
import { fmt } from "../lib/format";

type App = {
  id: string;
  namespace: string;
  created_at: string;
};

export function Apps({ refresh, bump }: { refresh: number; bump: () => void }) {
  const { send } = useAdmin();
  const [rows, setRows] = useState<App[]>([]);
  const [loading, setLoading] = useState(false);
  const [busy, setBusy] = useState(false);
  const [created, setCreated] = useState<{ applicationId: string; token: string } | null>(null);
  const [showCreate, setShowCreate] = useState(false);
  const [name, setName] = useState("");
  const [namespace, setNamespace] = useState("");
  const [rotateFor, setRotateFor] = useState<App | null>(null);
  const [rotated, setRotated] = useState<{ tokenId: string; token: string } | null>(null);

  const load = useCallback(async () => {
    setLoading(true);
    const r = await send({ op: "dashboard.list", name: "apps" });
    if (r.ok) setRows((r.data as App[]) ?? []);
    setLoading(false);
  }, [send]);

  useEffect(() => {
    void load();
  }, [load, refresh]);

  async function create() {
    setBusy(true);
    const r = await send({ op: "app.create", name, namespace });
    setBusy(false);
    if (r.ok) {
      setCreated(r.data as { applicationId: string; token: string });
      setShowCreate(false);
      setName("");
      setNamespace("");
      void load();
      bump();
    }
  }

  async function rotate() {
    if (!rotateFor) return;
    setBusy(true);
    const r = await send({ op: "app.token.rotate", applicationId: rotateFor.id });
    setBusy(false);
    if (r.ok) setRotated(r.data as { tokenId: string; token: string });
  }
  const columns: Column<App>[] = [
    { key: "namespace", label: "Namespace", render: (a) => <span className="font-medium">{a.namespace}</span> },
    { key: "id", label: "Application ID", render: (a) => <code className="text-xs">{a.id}</code> },
    { key: "created", label: "Created", render: (a) => <span className="text-xs text-base-content/60">{fmt(a.created_at)}</span> },
    {
      key: "actions",
      label: "",
      className: "text-right",
      render: (a) => (
        <button className="btn btn-ghost btn-xs" onClick={() => { setRotateFor(a); setRotated(null); }}>
          rotate token
        </button>
      ),
    },
  ];

  return (
    <div className="space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h2 className="text-xl font-semibold">Applications</h2>
          <p className="text-sm text-base-content/60">Registered clients and their scoped tokens.</p>
        </div>
        <button className="btn btn-primary btn-sm" onClick={() => setShowCreate(true)}>
          + Register application
        </button>
      </header>

      <DataTable columns={columns} rows={rows} loading={loading} empty="No applications registered." />

      {showCreate && (
        <dialog className="modal modal-open">
          <div className="modal-box">
            <h3 className="font-semibold text-lg">Register application</h3>
            <p className="py-2 text-sm text-base-content/60">
              Namespace must be unique. Token shown once.
            </p>
            <label className="form-control w-full">
              <span className="label-text mb-1">Name</span>
              <input className="input input-bordered" value={name} onChange={(e) => setName(e.target.value)} />
            </label>
            <label className="form-control mt-3 w-full">
              <span className="label-text mb-1">Namespace</span>
              <input className="input input-bordered font-mono" value={namespace} onChange={(e) => setNamespace(e.target.value)} />
            </label>
            <div className="modal-action">
              <button className="btn btn-ghost" onClick={() => setShowCreate(false)}>Cancel</button>
              <button className="btn btn-primary" disabled={busy || !name || !namespace} onClick={create}>
                {busy && <span className="loading loading-spinner loading-sm" />}Create
              </button>
            </div>
          </div>
          <div className="modal-backdrop" onClick={() => setShowCreate(false)} />
        </dialog>
      )}

      {created && (
        <dialog className="modal modal-open">
          <div className="modal-box">
            <h3 className="font-semibold text-lg">Application created</h3>
            <p className="py-2 text-sm text-warning">
              Copy the token now — it is stored as a hash and cannot be shown again.
            </p>
            <div className="space-y-3">
              <div>
                <span className="text-xs uppercase tracking-wider text-base-content/50">Application ID</span>
                <pre className="rounded-box bg-base-300 p-3 text-xs">{created.applicationId}</pre>
              </div>
              <div>
                <span className="text-xs uppercase tracking-wider text-base-content/50">Token</span>
                <pre className="rounded-box bg-base-300 p-3 text-xs break-all">{created.token}</pre>
              </div>
            </div>
            <div className="modal-action">
              <button className="btn btn-primary" onClick={() => setCreated(null)}>Done</button>
            </div>
          </div>
        </dialog>
      )}

      {rotateFor && (
        <dialog className="modal modal-open">
          <div className="modal-box">
            <h3 className="font-semibold text-lg">Rotate token — {rotateFor.namespace}</h3>
            {!rotated ? (
              <>
                <p className="py-2 text-sm text-base-content/60">
                  Creates an additional token. Keep the old token until every instance uses the new one, then revoke it.
                </p>
                <div className="modal-action">
                  <button className="btn btn-ghost" onClick={() => setRotateFor(null)}>Cancel</button>
                  <button className="btn btn-primary" disabled={busy} onClick={rotate}>
                    {busy && <span className="loading loading-spinner loading-sm" />}Rotate
                  </button>
                </div>
              </>
            ) : (
              <>
                <p className="py-2 text-sm text-warning">Copy the new token now.</p>
                <pre className="rounded-box bg-base-300 p-3 text-xs break-all">{rotated.token}</pre>
                <p className="py-2 text-xs text-base-content/60">
                  Revoke the previous token from the API once every instance uses this one. This
                  view cannot revoke it for you — the old token id is not returned by rotation.
                </p>
                <div className="modal-action">
                  <button className="btn btn-primary" onClick={() => setRotateFor(null)}>Done</button>
                </div>
              </>
            )}
          </div>
          <div className="modal-backdrop" onClick={() => setRotateFor(null)} />
        </dialog>
      )}
    </div>
  );
}
