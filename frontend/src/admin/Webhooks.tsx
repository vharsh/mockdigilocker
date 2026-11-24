import { useEffect, useState } from 'react';
import { admin } from '../lib/api';
import { Badge, Button, Card, Code, Empty, Loading, PageHeader, Select, Toggle, toast, useAsync } from '../components/ui';
import { fmtDate, statusTone } from '../lib/format';
import { useCanWrite } from './AdminApp';

export function Webhooks() {
  const { data, loading, reload } = useAsync(() => admin('GET', '/webhooks'));
  const clients = useAsync(() => admin('GET', '/clients'));
  const [open, setOpen] = useState<string | null>(null);
  const [live, setLive] = useState(true);
  const [testClient, setTestClient] = useState('');
  const canWrite = useCanWrite();
  useEffect(() => { if (!live) return; const t = setInterval(reload, 3000); return () => clearInterval(t); }, [live, reload]);
  const redeliver = async (id: string) => { try { await admin('POST', `/webhooks/${id}/redeliver`); toast.ok('Redelivery queued'); setTimeout(reload, 600); } catch (e) { toast.err(e); } };
  const test = async () => { try { await admin('POST', '/webhooks/test', { client_id: testClient }); toast.ok('Test event sent'); setTimeout(reload, 600); } catch (e) { toast.err(e); } };
  const withCb = (clients.data?.items || []).filter((c: any) => c.callback_url);
  return (
    <div>
      <PageHeader title="Webhooks & callbacks" subtitle="Signed callbacks for authentication, consent, token and document events, with exponential-backoff retries. Simulate failures with the webhook_failure scenario."
        actions={<label className="flex items-center gap-2 text-sm text-slate-600"><Toggle checked={live} onChange={setLive} label="Live" /> Live</label>} />
      {canWrite && (
        <Card title="Send a test event" className="mb-6">
          {withCb.length === 0 ? <p className="text-sm text-slate-500">No client has a callback URL. Set one under Integration clients.</p> : (
            <div className="flex flex-wrap gap-2"><Select value={testClient} onChange={(e) => setTestClient(e.target.value)} className="max-w-sm" aria-label="Client"><option value="">Choose client…</option>{withCb.map((c: any) => <option key={c.client_id} value={c.client_id}>{c.name} → {c.callback_url}</option>)}</Select><Button variant="console" disabled={!testClient} onClick={test}>Send test</Button></div>
          )}
          <p className="mt-3 text-xs text-slate-500">Verify with <code>X-MockDL-Signature: sha256=hex(HMAC_SHA256(webhook_secret || client_secret, raw_body))</code>.</p>
        </Card>
      )}
      <Card title="Event log">
        {loading && !data ? <Loading /> : data.items.length === 0 ? <Empty title="No webhook events yet" /> : (
          <ul className="divide-y divide-slate-100">
            {data.items.map((ev: any) => (
              <li key={ev.id} className="py-3">
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <button className="flex flex-wrap items-center gap-2 text-left" onClick={() => setOpen(open === ev.id ? null : ev.id)}>
                    <Badge tone={statusTone(ev.status)}>{ev.status}</Badge><code className="text-sm font-semibold">{ev.type}</code>
                    <span className="text-xs text-slate-500">{ev.client_id} · {ev.attempts.length} attempt(s) · {fmtDate(ev.created_at, true)}</span>
                    {ev.next_retry_at && <span className="text-xs text-amber-700">next retry {new Date(ev.next_retry_at).toLocaleTimeString()}</span>}
                  </button>
                  {canWrite && <Button size="sm" variant="secondary" onClick={() => redeliver(ev.id)}>Redeliver</Button>}
                </div>
                {open === ev.id && (
                  <div className="mt-3 grid gap-4 lg:grid-cols-2">
                    <div><div className="mb-1 text-xs font-semibold uppercase text-slate-500">POST {ev.url}</div><Code>{JSON.stringify(ev.payload, null, 2)}</Code></div>
                    <div><div className="mb-1 text-xs font-semibold uppercase text-slate-500">Attempts</div>
                      <ol className="flex flex-col gap-2 text-xs">{ev.attempts.map((a: any) => (
                        <li key={a.n} className="rounded border border-slate-200 p-2"><div className="flex justify-between"><span>#{a.n} · {new Date(a.at).toLocaleTimeString()} · {a.duration_ms} ms</span><Badge tone={a.status_code >= 200 && a.status_code < 300 ? 'green' : 'red'}>{a.status_code || 'error'}</Badge></div>
                          {a.error && <div className="mt-1 text-rose-700">{a.error}</div>}{a.response && <div className="mt-1 truncate font-mono text-slate-500">{a.response}</div>}</li>
                      ))}</ol>
                    </div>
                  </div>
                )}
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  );
}
