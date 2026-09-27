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
    if (!r.ok) {
      notify(r.error, false);
      after();
      return;
    }
    notify(r.data.replayed ? `Replayed ${d.event} to ${appName(d.application_id)}` : `Not replayed: delivery is ${d.status}`, r.data.replayed);
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
    if (!r.ok) {
      notify(r.error, false);
      after();
      return;
    }
    notify(r.data.cancelled ? `Cancelled ${d.event}` : `Not cancelled: delivery is ${d.status}`, r.data.cancelled);
    after();
  }

  return { replay, cancel };
}
