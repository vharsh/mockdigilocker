import { admin } from '../lib/api';
import { Link } from '../lib/router';
import { Alert, Badge, Card, Code, Loading, PageHeader, useAsync } from '../components/ui';

export function Overview() {
  const { data, error, loading } = useAsync(() => admin('GET', '/overview'));
  if (loading && !data) return <Loading />;
  if (error) return <Alert>{error.message}</Alert>;
  const c = data.counts;
  const tiles: [string, number | string, string][] = [
    ['Test users', c.users, '/admin/personas'], ['Documents', c.documents, '/admin/personas'], ['Clients', c.clients, '/admin/clients'],
    ['Active scenarios', c.scenarios_active, '/admin/scenarios'], ['Live tokens', c.live_tokens, '/admin/sessions'], ['Consents', c.consents, '/admin/sessions'],
    ['Webhook events', c.webhook_events, '/admin/webhooks'], ['Logged requests', c.request_logs, '/admin/inspector'],
  ];
  const h = data.last_hour;
  return (
    <div>
      <PageHeader title="Overview" subtitle={<>Environment <Badge tone="violet">{data.env}</Badge> · v{data.version} · store <code>{data.store}</code> {data.store_ok ? '✓' : '✕'} · kv <code>{data.kv}</code> {data.kv_ok ? '✓' : '✕'}</>} />
      {c.scenarios_active > 0 && <Alert tone="amber" className="mb-6">{c.scenarios_active} fault-injection scenario(s) are active — API behaviour differs from the happy path. <Link to="/admin/scenarios" className="font-semibold underline">Review</Link></Alert>}
      <div className="grid grid-cols-2 gap-4 md:grid-cols-4">
        {tiles.map(([l, n, to]) => (
          <Link key={l} to={to} className="rounded-xl border border-slate-200 bg-white p-4 shadow-sm transition hover:border-amber-500">
            <div className="text-2xl font-bold">{n}</div><div className="text-sm text-slate-500">{l}</div>
          </Link>
        ))}
      </div>
      <div className="mt-6 grid gap-6 lg:grid-cols-2">
        <Card title="DigiLocker API traffic · last hour">
          <div className="grid grid-cols-3 gap-4 text-center">
            <div><div className="text-2xl font-bold">{h.api_requests}</div><div className="text-xs text-slate-500">requests</div></div>
            <div><div className={`text-2xl font-bold ${h.errors ? 'text-rose-600' : ''}`}>{h.errors}</div><div className="text-xs text-slate-500">errors (4xx/5xx)</div></div>
            <div><div className="text-2xl font-bold">{h.avg_latency_ms.toFixed(1)}</div><div className="text-xs text-slate-500">avg latency ms</div></div>
          </div>
        </Card>
        <Card title="Endpoints for your QA config">
          <Code>{Object.entries(data.endpoints).map(([k, v]) => `${k.padEnd(21)} ${v}`).join('\n')}</Code>
        </Card>
      </div>
      <Card title="Quick start" className="mt-6">
        <ol className="list-decimal space-y-1.5 pl-5 text-sm text-slate-700">
          <li>Register your app under <Link to="/admin/clients" className="font-semibold text-amber-700">Integration clients</Link> (client id, secret, exact redirect URIs, callback URL).</li>
          <li>Point your app's DigiLocker base URL at this server (see the endpoints above).</li>
          <li>Sign in on the consent screen with a <Link to="/admin/personas" className="font-semibold text-amber-700">test persona</Link> (OTP <code>123456</code>, PIN <code>123456</code>).</li>
          <li>Inject failures with the <Link to="/admin/scenarios" className="font-semibold text-amber-700">scenario simulator</Link> and watch traffic in the <Link to="/admin/inspector" className="font-semibold text-amber-700">API inspector</Link>.</li>
          <li>Return to a known state with <Link to="/admin/environment" className="font-semibold text-amber-700">Environment reset</Link>.</li>
        </ol>
      </Card>
    </div>
  );
}
