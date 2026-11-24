import { useState, type FormEvent } from 'react';
import { admin } from '../lib/api';
import { Alert, Badge, Button, Card, Code, CopyButton, Field, Input, Loading, Modal, PageHeader, Select, Textarea, Toggle, toast, useAsync } from '../components/ui';
import { useCanWrite } from './AdminApp';

export function Clients() {
  const { data, error, loading, reload } = useAsync(() => admin('GET', '/clients'));
  const [editing, setEditing] = useState<any>(null);
  const [cfg, setCfg] = useState<any>(null);
  const canWrite = useCanWrite();
  if (loading && !data) return <Loading />;
  const showConfig = async (id: string) => { try { setCfg(await admin('GET', `/clients/${id}/config`)); } catch (e) { toast.err(e); } };
  const rotate = async (id: string) => { if (!confirm('Rotate secret? Existing integrations will need the new value.')) return; try { await admin('POST', `/clients/${id}/rotate-secret`); toast.ok('Secret rotated'); reload(); } catch (e) { toast.err(e); } };
  const del = async (id: string) => { if (!confirm('Delete client ' + id + '?')) return; try { await admin('DELETE', '/clients/' + id); reload(); } catch (e) { toast.err(e); } };
  return (
    <div>
      <PageHeader title="Integration clients" subtitle="Partner applications (requesters). Each has its own credentials, redirect URIs, API profile, token lifetimes and webhook callback."
        actions={canWrite && <Button variant="console" onClick={() => setEditing({})}>+ Register client</Button>} />
      {error && <Alert>{error.message}</Alert>}
      <div className="grid gap-4 xl:grid-cols-2">
        {data?.items.map((c: any) => (
          <Card key={c.client_id} title={<span className="flex items-center gap-2">{c.name}{c.enabled ? <Badge tone="green">enabled</Badge> : <Badge tone="red">disabled</Badge>}{c.require_pkce && <Badge tone="blue">PKCE</Badge>}</span>}
            actions={<>
              <Button size="sm" variant="secondary" onClick={() => showConfig(c.client_id)}>QA config</Button>
              {canWrite && <><Button size="sm" variant="secondary" onClick={() => setEditing(c)}>Edit</Button><Button size="sm" variant="ghost" onClick={() => rotate(c.client_id)}>Rotate secret</Button><Button size="sm" variant="ghost" onClick={() => del(c.client_id)}>Delete</Button></>}
            </>}>
            <dl className="grid grid-cols-[120px_1fr] gap-x-3 gap-y-2 text-sm">
              <dt className="text-slate-500">client_id</dt><dd className="font-mono">{c.client_id} <CopyButton text={c.client_id} /></dd>
              <dt className="text-slate-500">client_secret</dt><dd className="font-mono text-xs">{c.client_secret} <CopyButton text={c.client_secret} /></dd>
              <dt className="text-slate-500">redirect URIs</dt><dd className="flex flex-col gap-0.5 font-mono text-xs">{c.redirect_uris.map((u: string) => <span key={u} className="break-all">{u}</span>)}</dd>
              <dt className="text-slate-500">API profile</dt><dd><Badge tone="violet">{c.profile || 'digilocker'}</Badge></dd>
              <dt className="text-slate-500">callback URL</dt><dd className="break-all font-mono text-xs">{c.callback_url || '—'}</dd>
              <dt className="text-slate-500">lifetimes</dt><dd className="text-xs">access {c.access_token_ttl || 'default'}s · refresh {c.refresh_token_ttl || 'default'}s · code {c.code_ttl || 'default'}s · consent {c.consent_ttl || 'default'}s</dd>
            </dl>
          </Card>
        ))}
      </div>
      <Modal open={!!editing} onClose={() => setEditing(null)} title={editing?.client_id ? 'Edit ' + editing.client_id : 'Register client'} wide>
        {editing && <ClientForm client={editing} profiles={data.profiles} events={data.webhook_events} onSaved={() => { setEditing(null); reload(); }} />}
      </Modal>
      <Modal open={!!cfg} onClose={() => setCfg(null)} title="Configuration for your QA build" wide>
        {cfg && <>
          <p className="mb-2 text-sm text-slate-600">Drop these into the environment of the application under test:</p>
          <Code>{Object.entries(cfg.env).map(([k, v]) => `${k}=${v}`).join('\n')}</Code>
          <p className="mb-2 mt-4 text-sm text-slate-600">Sample authorize URL:</p>
          <Code>{cfg.sample_authorize_url}</Code>
        </>}
      </Modal>
    </div>
  );
}

function ClientForm({ client, profiles, events, onSaved }: { client: any; profiles: any[]; events: string[]; onSaved: () => void }) {
  const [f, setF] = useState<any>({
    client_id: '', client_secret: '', name: '', description: '', enabled: true, require_pkce: false, profile: 'digilocker', callback_url: '', webhook_secret: '',
    webhook_events: [], access_token_ttl: 0, refresh_token_ttl: 0, code_ttl: 0, consent_ttl: 0, scopes: ['openid'], ...client,
    redirect_text: (client.redirect_uris || []).join('\n'),
  });
  const [err, setErr] = useState('');
  const set = (k: string) => (e: any) => setF({ ...f, [k]: e.target.value });
  const num = (k: string) => (e: any) => setF({ ...f, [k]: Number(e.target.value) || 0 });
  const submit = async (e: FormEvent) => {
    e.preventDefault(); setErr('');
    const body = { ...f, redirect_uris: f.redirect_text.split(/\s+/).filter(Boolean) };
    delete body.redirect_text;
    try { await admin(client.client_id ? 'PUT' : 'POST', '/clients' + (client.client_id ? '/' + client.client_id : ''), body); toast.ok('Client saved'); onSaved(); }
    catch (e) { setErr(e instanceof Error ? e.message : String(e)); }
  };
  return (
    <form onSubmit={submit} className="grid gap-4 sm:grid-cols-2">
      {err && <Alert className="sm:col-span-2">{err}</Alert>}
      <Field label="Application name"><Input value={f.name} onChange={set('name')} required /></Field>
      <Field label="client_id" hint={client.client_id ? 'cannot be changed' : 'blank = generate; or reuse your real client id'}><Input value={f.client_id} onChange={set('client_id')} disabled={!!client.client_id} /></Field>
      <Field label="client_secret" hint="blank = generate / keep"><Input value={f.client_secret} onChange={set('client_secret')} /></Field>
      <Field label="API profile" hint="response/path compatibility mapping"><Select value={f.profile} onChange={set('profile')}>{profiles.map((p) => <option key={p.name} value={p.name}>{p.name}</option>)}</Select></Field>
      <Field label="Redirect URIs (one per line, exact match)" className="sm:col-span-2"><Textarea rows={3} value={f.redirect_text} onChange={set('redirect_text')} required /></Field>
      <Field label="Webhook callback URL"><Input value={f.callback_url} onChange={set('callback_url')} placeholder="https://qa.myapp.test/digilocker/webhook" /></Field>
      <Field label="Webhook signing secret" hint="blank = use client_secret"><Input value={f.webhook_secret} onChange={set('webhook_secret')} /></Field>
      <div className="sm:col-span-2">
        <div className="mb-1 text-xs font-semibold text-slate-600">Webhook events (none selected = all)</div>
        <div className="flex flex-wrap gap-3">{events.map((ev) => (
          <label key={ev} className="flex items-center gap-1.5 text-xs"><input type="checkbox" checked={f.webhook_events?.includes(ev)}
            onChange={(e) => setF({ ...f, webhook_events: e.target.checked ? [...(f.webhook_events || []), ev] : f.webhook_events.filter((x: string) => x !== ev) })} /><code>{ev}</code></label>
        ))}</div>
      </div>
      <Field label="Access token TTL (s, 0=default)"><Input type="number" min={0} value={f.access_token_ttl} onChange={num('access_token_ttl')} /></Field>
      <Field label="Refresh token TTL (s)"><Input type="number" min={0} value={f.refresh_token_ttl} onChange={num('refresh_token_ttl')} /></Field>
      <Field label="Auth code TTL (s)"><Input type="number" min={0} value={f.code_ttl} onChange={num('code_ttl')} /></Field>
      <Field label="Consent validity (s)"><Input type="number" min={0} value={f.consent_ttl} onChange={num('consent_ttl')} /></Field>
      <div className="flex items-center gap-6 sm:col-span-2">
        <label className="flex items-center gap-2 text-sm"><Toggle checked={f.enabled} onChange={(v) => setF({ ...f, enabled: v })} label="Enabled" /> Enabled</label>
        <label className="flex items-center gap-2 text-sm"><Toggle checked={f.require_pkce} onChange={(v) => setF({ ...f, require_pkce: v })} label="Require PKCE" /> Require PKCE</label>
      </div>
      <Button variant="console" type="submit" className="sm:col-span-2">Save client</Button>
    </form>
  );
}
