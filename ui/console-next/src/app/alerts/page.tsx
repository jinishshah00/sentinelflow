import Link from "next/link";
import { AlertsResponse, AlertT } from "@/types";

export const dynamic = "force-dynamic";

async function getAlerts(): Promise<AlertT[]> {
  const base = process.env.NEXT_INTERNAL_BASE || "http://localhost:3000";
  const res = await fetch(`${base}/api/sf/alerts?limit=50`, { cache: "no-store" });
  if (!res.ok) throw new Error("failed to load alerts");
  const data: AlertsResponse = await res.json();
  return data.alerts ?? [];
}

function badge(sev: string) {
  const cls = sev === "high" ? "bg-red-100 text-red-800"
    : sev === "medium" ? "bg-yellow-100 text-yellow-800"
    : "bg-green-100 text-green-800";
  return <span className={`px-2 py-1 rounded text-xs font-medium ${cls}`}>{sev}</span>;
}

export default async function AlertsPage() {
  const alerts = await getAlerts();
  return (
    <div>
      <h1 className="text-xl font-semibold mb-4">Alerts</h1>
      <div className="border rounded-md overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-gray-50">
            <tr>
              <th className="text-left px-3 py-2">ID</th>
              <th className="text-left px-3 py-2">Severity</th>
              <th className="text-left px-3 py-2">Status</th>
              <th className="text-left px-3 py-2">Event</th>
              <th className="text-left px-3 py-2">When</th>
            </tr>
          </thead>
          <tbody>
            {alerts.map((a) => (
              <tr key={a.alert_id} className="border-t hover:bg-gray-50">
                <td className="px-3 py-2">
                  <Link className="text-blue-600 hover:underline" href={`/alerts/${a.alert_id}`}>
                    {a.alert_id.slice(0, 8)}…
                  </Link>
                </td>
                <td className="px-3 py-2">{badge(a.triage.severity)}</td>
                <td className="px-3 py-2">{a.status}</td>
                <td className="px-3 py-2">{a.event.event_type}</td>
                <td className="px-3 py-2">{new Date(a.created).toLocaleString()}</td>
              </tr>
            ))}
            {alerts.length === 0 && (
              <tr>
                <td colSpan={5} className="px-3 py-6 text-center text-gray-500">No alerts yet.</td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
}
