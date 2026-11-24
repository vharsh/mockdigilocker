import { useEffect, useState } from 'react';
import { admin } from '../lib/api';
import { Badge, Button, Card, Code, Input, KV, Loading, Modal, PageHeader, Select, Toggle, toast, useAsync } from '../components/ui';
import { fmtDate, httpTone } from '../lib/format';
import { useCanWrite } from './AdminApp';

function pretty(s?: string) {
  if (!s) return '(empty)';
  try { return JSON.stringify(JSON.parse(s), null, 2); } catch { return s; }
}
const hdrs = (h: Record<string, string[]>) => Object.entries(h || {}).map(([k, v]) => `${k}: ${v.join(', ')}`).join('\n');

export function Inspector() {
  const [filters, setFilters] = useState({ category: 'api', status: '', q: '', client_id: '', trace_id: '' });
  const [live, setLive] = useState(true);
  const qs = new URLSearchParams(Object.entries(filters).filter(([, v]) => v)).toString();
  const { data, loading, reload } = useAsync(() => admin('GET', '/requests?limit=300&' + qs), [qs]);
  const [sel, setSel] = useState<string | null>(null);
  const canWrite = useCanWrite();
  useEffect(() => { if (!live) return; const t = setInterval(reload, 3000); return () => clearInterval(t); }, [live, reload]);
  const set = (k: string) => (e: any) => setFilters({ ...filters, [k]: e.target.value });
  const clear = async () => { if (!confirm('Clear all request logs?')) return; await admin('DELETE', '/requests'); toast.ok('Logs cleared'); reload(); };
  return (
    <div>
      <PageHeader title="API request inspector" subtitle="Every call to the DigiLocker-compatible API, consent pages and portal — headers, payloads, status, latency, trace & correlation ids, and the scenario that fired."
        actions={<><label className="flex items-center gap-2 text-sm text-slate-600"><Toggle checked={live} onChange={setLive} label="Live" /> Live</label>{canWrite && <Button variant="secondary" onClick={clear}>Clear logs</Button>}</>} />
      <div className="mb-4 grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
        <Select value={filters.category} onChange={set('category')} aria-label="Category"><option value="">All traffic</option><option value="api">DigiLocker API</option><option value="consent-ui">Consent pages</option><option value="portal">Portal API</option><option value="demo-partner">Demo partner</option><option value="admin">Console (writes)</option></Select>
        <Select value={filters.status} onChange={set('status')} aria-label="Status"><option value="">Any status</option><option value="ok">Success</option><option value="error">Errors only</option></Select>
        <Input placeholder="Path / error / scenario contains…" value={filters.q} onChange={set('q')} />
        <Input placeholder="client_id" value={filters.client_id} onChange={set('client_id')} />
        <Input placeholder="trace / correlation id" value={filters.trace_id} onChange={set('trace_id')} />
      </div>
      <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white shadow-sm">
        {loading && !data ? <Loading /> : (
          <table className="w-full text-sm">
            <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500"><tr><th className="px-3 py-2">Time</th><th className="px-3">Method / path</th><th className="px-3">Status</th><th className="px-3">Latency</th><th className="px-3">Client</th><th className="px-3">Scenario / error</th></tr></thead>
            <tbody className="divide-y divide-slate-100 font-mono text-xs">
              {data?.items.map((l: any) => (
                <tr key={l.id} className="cursor-pointer hover:bg-amber-50/60" onClick={() => setSel(l.id)}>
                  <td className="whitespace-nowrap px-3 py-2 text-slate-500">{new Date(l.timestamp).toLocaleTimeString()}</td>
                  <td className="max-w-md truncate px-3"><span className="font-semibold">{l.method}</span> {l.path}</td>
                  <td className="px-3"><Badge tone={httpTone(l.status)}>{l.status || 'reset'}</Badge></td>
                  <td className="px-3">{l.latency_ms.toFixed(1)} ms</td>
                  <td className="px-3">{l.client_id || '—'}</td>
                  <td className="max-w-xs truncate px-3 text-slate-600">{l.scenario && <span className="text-amber-700">⚡ {l.scenario} </span>}{l.error}</td>
                </tr>
              ))}
              {data?.items.length === 0 && <tr><td colSpan={6} className="py-10 text-center font-sans text-slate-500">No requests match. Run a flow from your app or the <a href="/demo-partner" className="text-amber-700 underline">demo partner</a>.</td></tr>}
            </tbody>
          </table>
        )}
      </div>
      <Modal open={!!sel} onClose={() => setSel(null)} title="Request details" wide>{sel && <RequestDetail id={sel} onTrace={(t) => { setSel(null); setFilters({ ...filters, category: '', trace_id: t }); }} />}</Modal>
    </div>
  );
}

function RequestDetail({ id, onTrace }: { id: string; onTrace: (t: string) => void }) {
  const { data: l, loading } = useAsync(() => admin('GET', '/requests/' + id), [id]);
  if (loading || !l) return <Loading />;
  const curl = `curl -i -X ${l.method} '${location.origin}${l.path}${l.query ? '?' + l.query : ''}'` +
    Object.entries(l.request_headers || {}).filter(([k]) => !/^(Accept-Encoding|Connection|Content-Length|User-Agent)$/i.test(k)).map(([k, v]: any) => ` \\\n  -H '${k}: ${v[0]}'`).join('') +
    (l.request_body && !l.request_body.startsWith('<binary') ? ` \\\n  --data '${l.request_body.replaceAll("'", "'\\''")}'` : '');
  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center gap-2"><Badge tone={httpTone(l.status)}>{l.status || 'connection reset'}</Badge><code className="font-semibold">{l.method} {l.path}</code></div>
      <KV items={[['Timestamp', fmtDate(l.timestamp, true)], ['Latency', l.latency_ms + ' ms'], ['Trace id', <button className="font-mono text-xs text-amber-700 underline" onClick={() => onTrace(l.trace_id)}>{l.trace_id}</button>],
        ['Correlation id', <code className="text-xs">{l.correlation_id}</code>], ['Span id', <code className="text-xs">{l.span_id}</code>], ['Client / user', <>{l.client_id || '—'} / <code className="text-xs">{l.user_id || '—'}</code></>],
        ['Scenario', l.scenario], ['Error', l.error], ['Endpoint', l.endpoint], ['Rewritten by profile', l.rewritten]]} />
      {l.notes?.length > 0 && <div className="rounded-lg bg-amber-50 p-3 text-xs text-amber-900">{l.notes.map((n: string) => <div key={n}>• {n}</div>)}</div>}
      <div className="grid gap-4 lg:grid-cols-2">
        <div><h4 className="mb-1 text-xs font-semibold uppercase text-slate-500">Request headers</h4><Code>{hdrs(l.request_headers) || '(none)'}</Code>
          {l.query && <><h4 className="mb-1 mt-3 text-xs font-semibold uppercase text-slate-500">Query</h4><Code>{decodeURIComponent(l.query.replaceAll('&', '\n'))}</Code></>}
          <h4 className="mb-1 mt-3 text-xs font-semibold uppercase text-slate-500">Request body</h4><Code>{pretty(l.request_body)}</Code></div>
        <div><h4 className="mb-1 text-xs font-semibold uppercase text-slate-500">Response headers</h4><Code>{hdrs(l.response_headers)}</Code>
          <h4 className="mb-1 mt-3 text-xs font-semibold uppercase text-slate-500">Response body</h4><Code>{pretty(l.response_body)}</Code></div>
      </div>
      <div><h4 className="mb-1 text-xs font-semibold uppercase text-slate-500">Replay with curl</h4><Code>{curl}</Code></div>
    </div>
  );
}
