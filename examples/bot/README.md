# Calcron Discord bot

A working Discord bot built on [`@bzrlab/calcron`](https://www.npmjs.com/package/@bzrlab/calcron) that uses every Calcron feature. By default it talks to the hosted instance at `wss://calcron.bzr.lt/ws`.

| Feature | Command | Calcron feature |
|---|---|---|
| Reminders | `/remind` with snooze and cancel buttons | Schedule, extend, cancel |
| Support tickets | `/ticket open`, `/ticket close` | Schedule with a chain: an idle warning, then an auto-close |
| Temporary roles | `/temprole add`, `extend`, `remove` | Schedule, extend, cancel |
| Staff alerts | `/alert` | Throttle: one alert per cooldown, sent on the leading edge |
| Giveaways | `/giveaway start`, `extend`, `end`, `cancel`, `reroll` | Absolute-time schedule |
| Member verification | Runs automatically when a member joins | Workflow with signals, a correlation key per join, a branch and a timeout |
| Daily standup | Runs on workdays; `/standup` starts one now | Business calendar, start schedule, missed occurrence policy |
| Operations | `/calcron stats`, `blocked`, `calendar`, `holiday` | Admin dashboard, delivery replay and cancel, calendar overrides |

Every delivery is acknowledged only after Discord confirms the side effect. Handlers are idempotent, so a redelivery after a crash never posts twice.

## Requirements

- Node 22.18 or newer. It runs the TypeScript directly, with no build step.
- A Discord application with the **Server Members** privileged intent enabled.
- Bot permissions: Manage Roles, Kick Members, Send Messages, Embed Links, Add Reactions, Read Message History, Create Private Threads, Create Public Threads, Send Messages in Threads, Manage Threads.
- Calcron admin token. It is only needed on the machine that provisions.

## Setup

```sh
npm install
cp .env.example .env
```

1. Fill in `DISCORD_TOKEN`, `DISCORD_GUILD_ID` and the channel and role IDs in `.env`.
2. Set `CALCRON_ADMIN_TOKEN`, then create the application:
   ```sh
   npm run provision -- app discord-bot
   ```
   Copy the printed `CALCRON_APP_ID` and `CALCRON_TOKEN` into `.env`. The token is shown only once.
3. Publish the workflows, the business calendar and the standup start schedule:
   ```sh
   npm run provision
   ```
   You can re-run this safely. Unchanged workflows are not re-published, and holiday overrides are kept.
4. Start the bot:
   ```sh
   npm start
   ```

The admin token controls every application on the Calcron instance. Keep it on the operator machine and leave it empty on the production bot host. The `/calcron` commands need it to be set.

## Token rotation

```sh
npm run provision -- rotate           # prints a new CALCRON_TOKEN
npm run provision -- revoke <tokenId> # after every bot instance uses the new token
```

## Tests

```sh
npm test
```

This runs the typecheck and the unit tests. The end-to-end test runs only when a Calcron server is available:

```sh
CALCRON_E2E_URL=ws://127.0.0.1:8080/ws CALCRON_E2E_ADMIN_TOKEN=<admin token> npm test
```
