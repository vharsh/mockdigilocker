import { useState } from 'react';
import { admin } from '../lib/api';
import { Alert, Badge, Button, Card, Code, Field, Input, Loading, PageHeader, Textarea, Toggle, toast, useAsync } from '../components/ui';
import { useCanWrite } from './AdminApp';

const targets: [string, string][] = [
  ['users', 'Reset all test users (reseed predefined personas + documents, delete custom ones)'],
  ['documents', 'Reset documents of predefined personas'],
  ['consents', 'Clear consent records'],
  ['sessions', 'Clear authorization requests, codes, OAuth tokens, portal sessions, OTPs'],
  ['scenarios', 'Reset scenario configuration (happy path)'],
  ['logs', 'Clear API request logs'],
  ['webhooks', 'Clear webhook event log'],
  ['otps', 'Clear OTP outbox'],
  ['activity', 'Clear user activity history'],
  ['clients', 'Restore default integration clients (removes custom ones)'],
  ['issuers', 'Restore issuers (all online)'],
];

export function Environment() {
  const cfg = useAsync(() => admin('GET', '/config'));
  const issuers = useAsync(() => admin('GET', '/issuers'));
  const profiles = useAsync(() => admin('GET', '/profiles'));
  const [sel, setSel] = useState<string[]>(['consents', 'sessions', 'scenarios', 'logs']);
  const [result, setResult] = useState<any>(null);
  const canWrite = useCanWrite();
  const reset = async (t: string[]) => {
    if (!confirm(`Reset: ${t.join(', ')}?`)) return;
    try { setResult(await admin('POST', '/reset', { targets: t })); toast.ok('Environment reset'); issuers.reload(); } catch (e) { toast.err(e); }
  };
  const setIssuer = async (orgid: string, available: boolean) => { try { await admin('PUT', '/issuers/' + orgid, { available }); issuers.reload(); toast.ok(available ? 'Issuer online' : 'Issuer marked unavailable'); } catch (e) { toast.err(e); } };
  return (
    <div>
      <PageHeader title="Environment & reset" subtitle="Return the sandbox to a known state, toggle issuer availability, manage API compatibility profiles, and review configuration." />
      <div className="grid gap-6 lg:grid-cols-2">
        <Card title="Test environment reset" actions={canWrite && <Button variant="danger" size="sm" onClick={() => reset(['all'])}>Reset everything</Button>}>
          <div className="flex flex-col gap-2">{targets.map(([k, l]) => (
            <label key={k} className="flex items-start gap-2 text-sm"><input type="checkbox" className="mt-1" checked={sel.includes(k)} onChange={(e) => setSel(e.target.checked ? [...sel, k] : sel.filter((x) => x !== k))} />
              <span><code className="font-semibold">{k}</code> — <span className="text-slate-600">{l}</span></span></label>
          ))}</div>
          {canWrite && <Button variant="console" className="mt-4" disabled={!sel.length} onClick={() => reset(sel)}>Reset selected</Button>}
          {result && <Code className="mt-4">{JSON.stringify(result, null, 2)}</Code>}
          <p className="mt-3 text-xs text-slate-500">Automate in CI: <code>POST /admin-api/reset {'{"targets":["all"]}'}</code> with an admin bearer token.</p>
        </Card>
        <Card title="Issuer availability">
          {!issuers.data ? <Loading /> : <ul className="divide-y divide-slate-100">{issuers.data.items.map((is: any) => (
            <li key={is.orgid} className="flex items-center justify-between py-3"><div><div className="text-sm font-semibold">{is.name}</div><div className="font-mono text-xs text-slate-500">{is.issuerid} · {is.doctypes.map((d: any) => d.doctype).join(', ')}</div></div>
              <div className="flex items-center gap-2"><Badge tone={is.available ? 'green' : 'red'}>{is.available ? 'online' : 'unavailable'}</Badge>{canWrite && <Toggle checked={is.available} onChange={(v) => setIssuer(is.orgid, v)} label="Available" />}</div></li>
          ))}</ul>}
          <p className="mt-2 text-xs text-slate-500">An unavailable issuer's documents disappear from <code>files/issued</code>, and file/XML/pull calls return <code>repository_service_inactive</code>.</p>
        </Card>
      </div>
      <ProfilesCard data={profiles.data} reload={profiles.reload} canWrite={canWrite} />
      <Card title="Effective configuration (secrets redacted)" className="mt-6">{cfg.data ? <Code>{JSON.stringify(cfg.data.config, null, 2)}</Code> : <Loading />}</Card>
    </div>
  );
}

const sample = `{
  "description": "Maps our legacy /api/dl/* paths and wraps responses",
  "prefix_rewrites": { "/api/dl/": "/public/oauth2/1/" },
  "path_aliases": { "/api/token": "/public/oauth2/1/token" },
  "dob_format": "YYYY-MM-DD",
  "error_format": "oauth",
  "rename": { "files_issued.items": { "uri": "document_uri" } },
  "remove": { "user": ["reference_key"] },
  "extra": { "token": { "consented_on": "sandbox" } },
  "wrap": { "user": "data" }
}`;

function ProfilesCard({ data, reload, canWrite }: { data: any; reload: () => void; canWrite: boolean }) {
  const [name, setName] = useState('my-integration');
  const [json, setJson] = useState(sample);
  const [err, setErr] = useState('');
  const save = async () => {
    setErr('');
    try { await admin('PUT', '/profiles/' + encodeURIComponent(name), JSON.parse(json)); toast.ok('Profile saved — assign it to a client'); reload(); } catch (e) { setErr(e instanceof Error ? e.message : String(e)); }
  };
  const del = async (n: string) => { try { await admin('DELETE', '/profiles/' + n); reload(); } catch (e) { toast.err(e); } };
  return (
    <Card title="API compatibility profiles" className="mt-6">
      <p className="mb-4 text-sm text-slate-600">Profiles map your integration's existing request paths and expected response shapes onto the mock — path aliases/prefix rewrites (global), and per-client response transforms (rename/remove/extra/wrap, date format, error format). Endpoint keys: {data?.endpoints?.map((e: string) => <code key={e} className="mr-1 rounded bg-slate-100 px-1 text-xs">{e}</code>)}</p>
      <div className="grid gap-6 lg:grid-cols-2">
        <div className="flex flex-col gap-3">{data?.items.map((p: any) => (
          <details key={p.name} className="rounded-lg border border-slate-200 p-3">
            <summary className="flex cursor-pointer items-center justify-between gap-2 text-sm font-semibold"><span>{p.name} {p.builtin && <Badge>built-in</Badge>}</span>{canWrite && !p.builtin && <button className="text-xs text-rose-600" onClick={(e) => { e.preventDefault(); del(p.name); }}>Delete</button>}</summary>
            <p className="mt-1 text-xs text-slate-600">{p.description}</p>
            <Code className="mt-2">{JSON.stringify(p, null, 2)}</Code>
            {canWrite && <button className="mt-2 text-xs font-semibold text-amber-700" onClick={() => { setName(p.builtin ? p.name + '-custom' : p.name); const c = { ...p }; delete c.name; delete c.builtin; setJson(JSON.stringify(c, null, 2)); }}>Use as template →</button>}
          </details>
        ))}</div>
        {canWrite && <div className="flex flex-col gap-3">
          {err && <Alert>{err}</Alert>}
          <Field label="Profile name"><Input value={name} onChange={(e) => setName(e.target.value)} /></Field>
          <Field label="Profile JSON"><Textarea rows={16} value={json} onChange={(e) => setJson(e.target.value)} spellCheck={false} /></Field>
          <Button variant="console" onClick={save}>Save profile</Button>
        </div>}
      </div>
    </Card>
  );
}
