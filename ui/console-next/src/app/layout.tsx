import "./globals.css";
import Link from "next/link";

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        <header className="border-b">
          <div className="mx-auto max-w-6xl px-4 py-3 flex items-center justify-between">
            <div className="font-bold text-xl">SentinelFlow Console</div>
            <nav className="space-x-4 text-sm">
              <Link href="/alerts" className="hover:underline">Alerts</Link>
              <Link href="/metrics" className="hover:underline">Metrics</Link>
            </nav>
          </div>
        </header>
        <main className="mx-auto max-w-6xl px-4 py-6">{children}</main>
      </body>
    </html>
  );
}
