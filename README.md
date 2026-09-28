# Calcron

Durable scheduler and workflow engine. Requires PostgreSQL and `CALCRON_ADMIN_TOKEN`.

```sh
cp .env.example .env   # fill in secrets
docker compose up -d --build
```

Compose serves Calcron through Traefik on the external `proxy` network. Open the host from the Traefik router rule for the admin dashboard. Apps connect to `wss://<host>/ws`.

## Application SDKs

Use the Node/TypeScript or Go client through the [SDK guide](docs/sdk.md). It
explains the command surface, delivery semantics, idempotency, and complete
examples.

## Admin dashboard

React + TypeScript + Tailwind/DaisyUI, built with Vite and embedded in the Go binary.

```sh
cd web
npm install
npm run build     # writes internal/server/dashboard/dist
cd .. && go build ./cmd/calcron
```

`npm run dev` serves the dashboard with the WebSocket proxied to a local Calcron on `:8080`.

Admin first sends `{"op":"auth","token":"..."}`, then `app.create`. It returns a one-time `cc_<id>_<secret>` application token.

Application commands: `schedule.set`, `schedule.cancel`, `schedule.extend`, `schedule.throttle`, `delivery.ack`, `workflow.start`, `signal`.

Admin commands: `workflow.publish`, `calendar.set`, `calendar.next`, `calendar.occurrences`, `start-schedule.set`, `dashboard.stats`, `dashboard.list`, `delivery.replay`, `delivery.cancel`, `app.token.rotate`, `app.token.revoke`.

Rotation creates an additional application token. Revoke old token only after every application instance uses replacement token.
