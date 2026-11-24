import { portal } from '../lib/api';
import { Alert, Badge, Button, Card, Empty, Loading, PageHeader, toast, useAsync } from '../components/ui';
import { fmtDate, statusTone, timeAgo } from '../lib/format';

export function Consents() {
  const { data, error, loading, reload } = useAsync(() => portal('GET', '/consents'));
  const revoke = async (id: string, name: string) => {
    if (!confirm(`Revoke ${name}'s access? Its tokens will stop working immediately.`)) return;
    try { await portal('POST', `/consents/${id}/revoke`); toast.ok('Access revoked'); reload(); } catch (e) { toast.err(e); }
  };
  return (
    <div>
      <PageHeader title="Shared with apps" subtitle="Consents you granted (or denied) to partner applications. Revoking invalidates their access tokens." />
      {error && <Alert>{error.message}</Alert>}
      {loading && !data ? <Loading /> : !data?.items.length ? <Card><Empty title="No consent records yet">Start an authorization from a partner app (or the demo partner) to see it here.</Empty></Card> : (
        <div className="flex flex-col gap-3">
          {data.items.map((c: any) => (
            <Card key={c.id}>
              <div className="flex flex-wrap items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex flex-wrap items-center gap-2"><span className="font-semibold">{c.client_name}</span><Badge tone={statusTone(c.status)}>{c.status}</Badge>{c.reason && <span className="text-xs text-slate-500">{c.reason.replaceAll('_', ' ')}</span>}</div>
                  <div className="mt-1 text-sm text-slate-600">
                    {c.granted_uris.length} document(s){c.share_eaadhaar && ' + Aadhaar data'}
                    {c.requested_doctypes?.length > 0 && <> · requested {c.requested_doctypes.join(', ')}</>}
                    {c.purpose && <> · purpose: {c.purpose}</>}
                  </div>
                  <div className="mt-1 text-xs text-slate-500">
                    Decided {fmtDate(c.decided_at, true)}{c.expires_at && <> · expires {fmtDate(c.expires_at, true)}</>}{c.revoked_at && <> · revoked {fmtDate(c.revoked_at, true)}</>}
                  </div>
                  {c.granted_uris.length > 0 && <div className="mt-2 flex flex-wrap gap-1">{c.granted_uris.map((u: string) => <code key={u} className="rounded bg-slate-100 px-1.5 py-0.5 text-[11px]">{u}</code>)}</div>}
                </div>
                {c.status === 'approved' && <Button variant="danger" size="sm" onClick={() => revoke(c.id, c.client_name)}>Revoke access</Button>}
              </div>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

const icons: Record<string, string> = {
  login: '→', consent_approved: '✓', consent_denied: '✕', consent_revoked: '⊘', document_shared: '⇪', document_pulled: '⇩',
  document_issued: '+', profile_updated: '✎', pin_changed: '✎', account_created: '★', token_revoked: '⊘', upload: '⇧',
};

export function ActivityList({ items }: { items: any[] }) {
  if (!items?.length) return <p className="text-sm text-slate-500">No activity yet.</p>;
  return (
    <ol className="flex flex-col">
      {items.map((a) => (
        <li key={a.id} className="flex gap-3 border-b border-slate-100 py-2.5 last:border-0">
          <span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-slate-100 text-xs text-slate-600">{icons[a.type] || '•'}</span>
          <div className="min-w-0 flex-1"><div className="text-sm text-slate-800">{a.message}</div><div className="text-xs text-slate-500">{fmtDate(a.created_at, true)} · {timeAgo(a.created_at)}</div></div>
        </li>
      ))}
    </ol>
  );
}

export function Activity() {
  const { data, error, loading } = useAsync(() => portal('GET', '/activity'));
  return (
    <div>
      <PageHeader title="Activity" subtitle="Logins, consents, document access by partner apps and profile changes." />
      {error && <Alert>{error.message}</Alert>}
      <Card>{loading && !data ? <Loading /> : <ActivityList items={data?.items} />}</Card>
    </div>
  );
}
