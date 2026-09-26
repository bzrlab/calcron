# Calcron

Private durable scheduler. Requires PostgreSQL and `CALCRON_ADMIN_TOKEN`.

```sh
docker compose up --build
```

Open `http://localhost:8080`. Apps connect to `ws://localhost:8080/ws`.

Admin first sends `{"op":"auth","token":"..."}`, then `app.create`. It returns a one-time `cc_<id>_<secret>` application token.

Application commands: `schedule.set`, `schedule.cancel`, `schedule.extend`, `schedule.throttle`, `delivery.ack`, `workflow.start`, `signal`.

Admin commands: `workflow.publish`, `calendar.set`, `start-schedule.set`, `dashboard.stats`, `dashboard.list`, `delivery.replay`, `delivery.cancel`, `app.token.rotate`, `app.token.revoke`.

Rotation creates an additional application token. Revoke old token only after every application instance uses replacement token.
