import { useState } from 'react';
import { admin } from '../lib/api';
import { Badge, Button, Card, Empty, Loading, PageHeader, Tabs, toast, useAsync } from '../components/ui';
import { fmtDate, statusTone, timeAgo } from '../lib/format';
import { useCanWrite } from './AdminApp';

export function Sessions() {
  const [tab, setTab] = useState<'consents' | 'tokens' | 'otps'>('consents');
  return (
    <div>
      <PageHeader title="Consents & sessions" subtitle="Inspect and manipulate consent records, live OAuth tokens, and the OTP outbox (where sandbox OTPs are 'delivered')." />
      <div className="mb-4 max-w-md"><Tabs tabs={[{ key: 'consents', label: 'Consents' }, { key: 'tokens', label: 'Live tokens' }, { key: 'otps', label: 'OTP outbox' }]} value={tab} onChange={setTab} /></div>
      {tab === 'consents' && <ConsentsTab />}{tab === 'tokens' && <TokensTab />}{tab === 'otps' && <OTPTab />}
    </div>
  );
}

function ConsentsTab() {
  const { data, loading, reload } = useAsync(() => admin('GET', '/consents'));
  const canWrite = useCanWrite();
  const act = async (id: string, what: 'revoke' | 'expire') => { try { await admin('POST', `/consents/${id}/${what}`); toast.ok(`Consent ${what}d`); reload(); } catch (e) { toast.err(e); } };
  if (loading && !data) return <Loading />;
  if (!data.items.length) return <Card><Empty title="No consents yet" /></Card>;
  return (
    <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white shadow-sm"><table className="w-full text-sm">
      <thead className="bg-slate-50 text-left text-xs uppercase text-slate-500"><tr><th className="px-4 py-2">Client</th><th>User</th><th>Status</th><th>Docs</th><th>Decided</th><th>Expires</th><th /></tr></thead>
      <tbody className="divide-y divide-slate-100">{data.items.map((c: any) => (
        <tr key={c.id}><td className="px-4 py-2">{c.client_name}<div className="font-mono text-[11px] text-slate-500">{c.id}</div></td><td>{c.user_name}</td>
          <td><Badge tone={statusTone(c.status)}>{c.status}</Badge>{c.reason && <div className="text-[11px] text-slate-500">{c.reason}</div>}</td>
          <td>{c.granted_uris.length}{c.share_eaadhaar && ' +eKYC'}</td><td className="text-xs">{fmtDate(c.decided_at, true)}</td><td className="text-xs">{fmtDate(c.expires_at, true)}</td>
          <td className="whitespace-nowrap text-right">{canWrite && c.status === 'approved' && <><Button size="sm" variant="ghost" onClick={() => act(c.id, 'expire')}>Expire now</Button><Button size="sm" variant="ghost" onClick={() => act(c.id, 'revoke')}>Revoke</Button></>}</td></tr>
      ))}</tbody></table></div>
  );
}

function TokensTab() {
  const { data, loading, reload } = useAsync(() => admin('GET', '/sessions'));
  const canWrite = useCanWrite();
  const act = async (t: string, what: 'expire' | 'revoke') => {
    try { await admin(what === 'expire' ? 'POST' : 'DELETE', `/tokens/${t}${what === 'expire' ? '/expire' : ''}`); toast.ok('Token ' + what + 'd'); reload(); } catch (e) { toast.err(e); }
  };
  if (loading && !data) return <Loading />;
  if (!data.items.length) return <Card><Empty title="No live OAuth tokens" /></Card>;
  return (
    <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white shadow-sm"><table className="w-full text-sm">
      <thead className="bg-slate-50 text-left text-xs uppercase text-slate-500"><tr><th className="px-4 py-2">Token</th><th>Kind</th><th>Client</th><th>User</th><th>Issued</th><th>Expires</th><th /></tr></thead>
      <tbody className="divide-y divide-slate-100">{data.items.map((t: any) => (
        <tr key={t.token}><td className="px-4 py-2 font-mono text-xs" title={t.token}>{t.masked}</td><td><Badge tone={t.kind === 'access' ? 'blue' : 'violet'}>{t.kind}</Badge></td><td>{t.client_id}</td>
          <td className="font-mono text-[11px]">{t.user_id.slice(0, 13)}…</td><td className="text-xs">{timeAgo(t.issued_at)}</td><td className="text-xs">{t.expired ? <Badge tone="red">expired</Badge> : fmtDate(t.expires_at, true)}</td>
          <td className="whitespace-nowrap text-right">{canWrite && <><Button size="sm" variant="ghost" onClick={() => act(t.token, 'expire')}>Expire</Button><Button size="sm" variant="ghost" onClick={() => act(t.token, 'revoke')}>Revoke</Button></>}</td></tr>
      ))}</tbody></table></div>
  );
}

function OTPTab() {
  const { data, loading } = useAsync(() => admin('GET', '/otps'));
  if (loading && !data) return <Loading />;
  return (
    <Card title={`OTP outbox (mode: ${data.mode})`}>
      {!data.items.length ? <Empty title="No OTPs issued yet" /> : (
        <table className="w-full text-sm"><thead className="text-left text-xs uppercase text-slate-500"><tr><th className="py-2">OTP</th><th>Sent to</th><th>Purpose</th><th>Sent</th><th>Expires</th></tr></thead>
          <tbody className="divide-y divide-slate-100">{data.items.map((o: any) => (
            <tr key={o.id}><td className="py-2 font-mono text-base font-bold tracking-widest">{o.otp}</td><td>{o.identifier} <span className="text-xs text-slate-500">({o.channel})</span></td><td><code className="text-xs">{o.purpose}</code></td>
              <td className="text-xs">{timeAgo(o.created_at)}</td><td className="text-xs">{new Date(o.expires_at) < new Date() ? <Badge tone="red">expired</Badge> : new Date(o.expires_at).toLocaleTimeString()}</td></tr>
          ))}</tbody></table>
      )}
    </Card>
  );
}
