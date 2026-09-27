import { useState } from "react";

export function Login({
  onConnect,
  error,
}: {
  onConnect: (token: string) => Promise<void>;
  error: string | null;
}) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!token) return;
    setBusy(true);
    try {
      await onConnect(token);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="min-h-screen grid place-items-center bg-base-200">
      <form
        onSubmit={submit}
        className="card w-full max-w-sm bg-base-100 border border-base-300 shadow-xl"
      >
        <div className="card-body gap-4">
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-box bg-primary text-primary-content font-bold">
              C
            </div>
            <div>
              <h1 className="text-lg font-semibold leading-tight">Calcron</h1>
              <p className="text-xs text-base-content/60">
                Durable scheduler operations
              </p>
            </div>
          </div>

          <label className="form-control w-full">
            <span className="label-text mb-1 text-sm">Admin token</span>
            <input
              type="password"
              autoFocus
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="CALCRON_ADMIN_TOKEN"
              className="input input-bordered w-full font-mono"
            />
          </label>

          {error && (
            <div role="alert" className="alert alert-error py-2 text-sm">
              {error}
            </div>
          )}

          <button
            type="submit"
            className="btn btn-primary w-full"
            disabled={busy || !token}
          >
            {busy && <span className="loading loading-spinner loading-sm" />}
            {busy ? "Authenticating…" : "Connect"}
          </button>
          <p className="text-center text-xs text-base-content/50">
            Token stays in this tab. Sent only as the first frame.
          </p>
          <details className="text-xs text-base-content/60">
            <summary className="cursor-pointer">Which token?</summary>
            <p className="mt-2">
              Use the administrator token, <code>CALCRON_ADMIN_TOKEN</code> from the server environment. Application
              tokens cannot open the dashboard; they belong to application SDKs and are issued from Applications.
            </p>
          </details>
        </div>
      </form>
    </div>
  );
}
