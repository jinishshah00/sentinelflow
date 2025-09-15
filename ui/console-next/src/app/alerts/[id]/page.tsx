import ApproveButton from "@/components/ApproveButton";
import { AlertT } from "@/types";

export const dynamic = "force-dynamic";

// v15: params is a Promise now
async function getAlert(id: string): Promise<AlertT> {
  const base = process.env.NEXT_INTERNAL_BASE || "http://localhost:3000";
  const res  = await fetch(`${base}/api/sf/alerts/${id}`, { cache: "no-store" });
  if (!res.ok) throw new Error("not found");
  return res.json();
}

function Row({ k, v }: { k: string; v: React.ReactNode }) {
  return (
    <div className="grid grid-cols-3 gap-2 py-1">
      <div className="text-gray-500">{k}</div>
      <div className="col-span-2">{v}</div>
    </div>
  );
}

export default async function AlertDetail(
  { params }: { params: Promise<{ id: string }> }   // <-- changed
) {
  const { id } = await params;                       // <-- await the promise
  const a = await getAlert(id);
  const labels  = a.event.labels ?? [];
  const reasons = a.triage.reason_tokens ?? [];

  return (
    <div className="space-y-6">
      <h1 className="text-xl font-semibold">Alert {a.alert_id}</h1>

      <div className="border rounded-md p-4 space-y-2">
        <Row k="Severity" v={<span className="font-medium">{a.triage.severity} (conf {a.triage.confidence.toFixed(2)})</span>} />
        <Row k="Status" v={<span>{a.status}</span>} />
        <Row k="Event Type" v={<code>{a.event.event_type}</code>} />
        <Row k="Principal" v={<code>{a.event.principal}</code>} />
        <Row k="Target" v={<code>{a.event.target}</code>} />
        <Row k="Labels" v={<code>{labels.length ? labels.join(", ") : "—"}</code>} />
        <Row k="Reason Tokens" v={<code>{reasons.length ? reasons.join(", ") : "—"}</code>} />
        <Row k="Description" v={<span>{a.event.description}</span>} />
        <Row k="Created" v={<span>{new Date(a.created).toLocaleString()}</span>} />
      </div>

      {a.status === "awaiting_approval" && (
        <div className="border rounded-md p-4">
          <div className="mb-2 font-medium">Action required</div>
          <ApproveButton id={a.alert_id} />
        </div>
      )}
    </div>
  );
}
