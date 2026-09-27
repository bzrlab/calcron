import { useAdmin } from "./admin";
import { useShell } from "./shell";

type DeliveryRef = { id: string; event: string; status: string; application_id: string };

export const canReplay = (d: DeliveryRef) => d.status === "blocked" || d.status === "cancelled";
export const canCancel = (d: DeliveryRef) => d.status === "pending" || d.status === "blocked";

export function useDeliveryActions(after: () => void) {
  const { send } = useAdmin();
  const { notify, confirm, appName } = useShell();

  async function replay(d: DeliveryRef) {
    const r = await send({ op: "delivery.replay", deliveryId: d.id });
    const done = r.ok && (r.data as { replayed?: boolean })?.replayed;
    notify(done ? `Replayed ${d.event} to ${appName(d.application_id)}` : r.error ?? `Not replayed: delivery is ${d.status}`, !!done);
    after();
  }

  async function cancel(d: DeliveryRef) {
    const yes = await confirm({
      title: `Cancel delivery ${d.id.slice(0, 10)}…?`,
      body: `${appName(d.application_id)} will not receive ${d.event}. Replay can restore it later.`,
      action: "Cancel delivery",
    });
    if (!yes) return;
    const r = await send({ op: "delivery.cancel", deliveryId: d.id });
    const done = r.ok && (r.data as { cancelled?: boolean })?.cancelled;
    notify(done ? `Cancelled ${d.event}` : r.error ?? `Not cancelled: delivery is ${d.status}`, !!done);
    after();
  }

  return { replay, cancel };
}
