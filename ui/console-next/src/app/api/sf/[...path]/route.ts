import { NextRequest } from "next/server";

const API_BASE = process.env.API_BASE!;
const API_KEY = process.env.API_KEY!;

// GET /api/sf/<path>
export async function GET(req: NextRequest, { params }: { params: { path: string[] } }) {
  const url = `${API_BASE}/${params.path.join("/")}${req.nextUrl.search}`;
  const res = await fetch(url, {
    headers: { "X-API-Key": API_KEY },
    cache: "no-store",
  });
  const body = await res.text();
  return new Response(body, {
    status: res.status,
    headers: { "content-type": res.headers.get("content-type") ?? "application/json" },
  });
}

// POST /api/sf/<path>
export async function POST(req: NextRequest, { params }: { params: { path: string[] } }) {
  const url = `${API_BASE}/${params.path.join("/")}${req.nextUrl.search}`;
  const body = await req.text();
  const res = await fetch(url, {
    method: "POST",
    headers: {
      "X-API-Key": API_KEY,
      "content-type": req.headers.get("content-type") ?? "application/json",
    },
    body,
    cache: "no-store",
  });
  const out = await res.text();
  return new Response(out, {
    status: res.status,
    headers: { "content-type": res.headers.get("content-type") ?? "application/json" },
  });
}
