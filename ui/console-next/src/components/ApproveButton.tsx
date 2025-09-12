"use client";

import { useState, useTransition } from "react";

export default function ApproveButton({ id }: { id: string }) {
  const [pending, start] = useTransition();
  const [msg, setMsg] = useState<string | null>(null);

  return (
    <div className="flex items-center gap-3">
      <button
        onClick={() => {
          setMsg(null);
          start(async () => {
            const res = await fetch(`/api/sf/alerts/${id}/approve`, { method: "POST" });
            if (res.ok) {
              setMsg("Approved. Refreshing…");
              location.reload();
            } else {
              setMsg(`Error: ${res.status}`);
            }
          });
        }}
        className="px-3 py-1 rounded bg-blue-600 text-white text-sm disabled:opacity-50"
        disabled={pending}
      >
        {pending ? "Approving…" : "Approve"}
      </button>
      {msg && <span className="text-xs text-gray-600">{msg}</span>}
    </div>
  );
}
