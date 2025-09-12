export const dynamic = "force-dynamic";

type M = { sample_window: number; counts: { Low: number; Med: number; High: number; Awaiting: number; Executed: number; Pending: number } };

export default async function MetricsPage() {
  const base = process.env.NEXT_INTERNAL_BASE || "http://localhost:3000";
  const res = await fetch(`${base}/api/sf/metrics`, { cache: "no-store" });
  if (!res.ok) throw new Error("metrics error");
  const m: M = await res.json();

  const Item = ({ label, value }: { label: string; value: number }) => (
    <div className="border rounded-md p-4">
      <div className="text-sm text-gray-500">{label}</div>
      <div className="text-2xl font-semibold">{value}</div>
    </div>
  );

  return (
    <div>
      <h1 className="text-xl font-semibold mb-4">Metrics (last {m.sample_window})</h1>
      <div className="grid grid-cols-2 md:grid-cols-3 gap-4">
        <Item label="High" value={m.counts.High} />
        <Item label="Medium" value={m.counts.Med} />
        <Item label="Low" value={m.counts.Low} />
        <Item label="Awaiting Approval" value={m.counts.Awaiting} />
        <Item label="Executed" value={m.counts.Executed} />
        <Item label="Pending" value={m.counts.Pending} />
      </div>
    </div>
  );
}
