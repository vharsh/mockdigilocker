import { useMemo, useState, type FormEvent } from 'react';
import { admin } from '../lib/api';
import { Alert, Badge, Button, Card, Field, Input, Loading, Modal, PageHeader, Select, Toggle, toast, useAsync } from '../components/ui';
import { useCanWrite } from './AdminApp';

const catTone: Record<string, any> = { authentication: 'blue', authorization: 'violet', documents: 'green', network: 'red', webhooks: 'amber' };

export function Scenarios() {
  const cat = useAsync(() => admin('GET', '/scenario-catalog'));
  const list = useAsync(() => admin('GET', '/scenarios'));
  const users = useAsync(() => admin('GET', '/users'));
  const clients = useAsync(() => admin('GET', '/clients'));
  const [form, setForm] = useState<any>(null);
  const canWrite = useCanWrite();
  if (!cat.data || !list.data) return <Loading />;
  const effects: any[] = cat.data.effects;
  const byKey = Object.fromEntries(effects.map((e) => [e.key, e]));
  const target = (s: any) => s.scope === 'user' ? users.data?.items.find((u: any) => u.id === s.target_id)?.name || s.target_id : s.scope === 'client' ? s.target_id : 'everyone';

  const toggle = async (s: any) => { try { await admin('PUT', '/scenarios/' + s.id, { ...s, enabled: !s.enabled }); list.reload(); } catch (e) { toast.err(e); } };
  const remove = async (s: any) => { try { await admin('DELETE', '/scenarios/' + s.id); list.reload(); } catch (e) { toast.err(e); } };
  const preset = async (k: string) => { try { const r = await admin('POST', '/scenarios/presets/' + k); toast.ok(`Preset applied (${r.scenarios} scenarios)`); list.reload(); } catch (e) { toast.err(e); } };

  return (
    <div>
      <PageHeader title="Scenario simulator" subtitle="Inject DigiLocker behaviours without changing application code. Scope globally, per test user, or per integration client — the most specific match wins (client › user › global)."
        actions={canWrite && <Button variant="console" onClick={() => setForm({ enabled: true, scope: 'global', effect: 'access_token_expired', endpoint: '*', probability: 1, latency_ms: 0, max_hits: 0, params: {} })}>+ New scenario</Button>} />
      {canWrite && (
        <Card title="One-click presets" className="mb-6">
          <div className="flex flex-wrap gap-2">{cat.data.presets.map((p: any) => <Button key={p.key} size="sm" variant={p.key === 'happy_path' ? 'secondary' : 'ghost'} className={p.key === 'happy_path' ? '' : 'ring-1 ring-slate-200'} onClick={() => preset(p.key)}>{p.label}</Button>)}</div>
          <p className="mt-2 text-xs text-slate-500">Presets replace all current scenarios.</p>
        </Card>
      )}
      <Card title={`Configured scenarios (${list.data.items.length})`}>
        {list.data.items.length === 0 ? <p className="text-sm text-slate-500">No scenarios — every endpoint behaves normally (happy path).</p> : (
          <div className="overflow-x-auto"><table className="w-full text-sm">
            <thead className="text-left text-xs uppercase tracking-wide text-slate-500"><tr><th className="py-2">On</th><th>Effect</th><th>Endpoint</th><th>Applies to</th><th>Options</th><th>Hits</th><th /></tr></thead>
            <tbody className="divide-y divide-slate-100">
              {list.data.items.map((s: any) => {
                const e = byKey[s.effect];
                return (
                  <tr key={s.id} className={s.enabled ? '' : 'opacity-50'}>
                    <td className="py-2.5"><Toggle checked={s.enabled} onChange={() => canWrite && toggle(s)} label="Enabled" /></td>
                    <td><div className="font-medium">{e?.label || s.effect}</div><div className="flex items-center gap-1"><Badge tone={catTone[e?.category] || 'slate'}>{e?.category}</Badge><code className="text-[11px] text-slate-500">{s.effect}</code></div></td>
                    <td><code>{s.endpoint}</code></td>
                    <td><Badge tone={s.scope === 'global' ? 'slate' : s.scope === 'user' ? 'blue' : 'violet'}>{s.scope}</Badge> <span className="text-xs">{target(s)}</span></td>
                    <td className="text-xs text-slate-600">{s.probability > 0 && s.probability < 1 && <div>{Math.round(s.probability * 100)}% of calls</div>}{s.latency_ms > 0 && <div>+{s.latency_ms} ms</div>}{s.max_hits > 0 && <div>first {s.max_hits} call(s)</div>}{Object.entries(s.params || {}).map(([k, v]) => <div key={k}><code>{k}={String(v)}</code></div>)}</td>
                    <td>{s.hits}</td>
                    <td className="whitespace-nowrap text-right">{canWrite && <><Button size="sm" variant="ghost" onClick={() => setForm(s)}>Edit</Button><Button size="sm" variant="ghost" onClick={() => remove(s)}>Delete</Button></>}</td>
                  </tr>
                );
              })}
            </tbody>
          </table></div>
        )}
      </Card>
      <Card title="Effect catalog" className="mt-6">
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
          {effects.map((e) => (
            <div key={e.key} className="rounded-lg border border-slate-200 p-3">
              <div className="flex items-center justify-between gap-2"><span className="text-sm font-semibold">{e.label}</span><Badge tone={catTone[e.category]}>{e.category}</Badge></div>
              <p className="mt-1 text-xs text-slate-600">{e.description}</p>
              <div className="mt-2 flex flex-wrap items-center gap-1 text-[11px] text-slate-500"><code>{e.key}</code>{e.status ? <span>· HTTP {e.status}</span> : null}{e.error && <code>· {e.error}</code>}</div>
              {canWrite && <button className="mt-2 text-xs font-semibold text-amber-700 hover:underline" onClick={() => setForm({ enabled: true, scope: 'global', effect: e.key, endpoint: e.endpoints[0], probability: 1, latency_ms: 0, max_hits: 0, params: {} })}>Use this effect →</button>}
            </div>
          ))}
        </div>
      </Card>
      <Modal open={!!form} onClose={() => setForm(null)} title={form?.id ? 'Edit scenario' : 'New scenario'} wide>
        {form && <ScenarioForm initial={form} effects={effects} users={users.data?.items || []} clients={clients.data?.items || []} onSaved={() => { setForm(null); list.reload(); }} />}
      </Modal>
    </div>
  );
}

function ScenarioForm({ initial, effects, users, clients, onSaved }: { initial: any; effects: any[]; users: any[]; clients: any[]; onSaved: () => void }) {
  const [f, setF] = useState<any>({ ...initial, params: { ...(initial.params || {}) } });
  const [err, setErr] = useState('');
  const eff = useMemo(() => effects.find((e) => e.key === f.effect), [f.effect, effects]);
  const groups = useMemo(() => effects.reduce((m: Record<string, any[]>, e) => ((m[e.category] ||= []).push(e), m), {}), [effects]);
  const submit = async (e: FormEvent) => {
    e.preventDefault(); setErr('');
    const body = { ...f, probability: Number(f.probability) || 0, latency_ms: Number(f.latency_ms) || 0, max_hits: Number(f.max_hits) || 0 };
    try { await admin(f.id ? 'PUT' : 'POST', '/scenarios' + (f.id ? '/' + f.id : ''), body); toast.ok('Scenario saved'); onSaved(); }
    catch (e) { setErr(e instanceof Error ? e.message : String(e)); }
  };
  return (
    <form onSubmit={submit} className="grid gap-4 sm:grid-cols-2">
      {err && <Alert className="sm:col-span-2">{err}</Alert>}
      <Field label="Effect" className="sm:col-span-2" hint={eff?.description}>
        <Select value={f.effect} onChange={(e) => { const ne = effects.find((x) => x.key === e.target.value); setF({ ...f, effect: e.target.value, endpoint: ne.endpoints.includes(f.endpoint) ? f.endpoint : ne.endpoints[0] }); }}>
          {Object.entries(groups).map(([g, list]) => <optgroup key={g} label={g}>{(list as any[]).map((e) => <option key={e.key} value={e.key}>{e.label} ({e.key})</option>)}</optgroup>)}
        </Select>
      </Field>
      <Field label="Injection point (endpoint)" hint="* = every DigiLocker API call"><Select value={f.endpoint} onChange={(e) => setF({ ...f, endpoint: e.target.value })}>{eff?.endpoints.map((x: string) => <option key={x}>{x}</option>)}</Select></Field>
      <Field label="Name (optional)"><Input value={f.name || ''} onChange={(e) => setF({ ...f, name: e.target.value })} placeholder={eff?.label} /></Field>
      <Field label="Scope"><Select value={f.scope} onChange={(e) => setF({ ...f, scope: e.target.value, target_id: '' })}><option value="global">Global (everyone)</option><option value="user">Per test user</option><option value="client">Per integration client</option></Select></Field>
      {f.scope === 'user' && <Field label="Test user"><Select value={f.target_id} onChange={(e) => setF({ ...f, target_id: e.target.value })} required><option value="">Select…</option>{users.map((u) => <option key={u.id} value={u.id}>{u.name} ({u.mobile})</option>)}</Select></Field>}
      {f.scope === 'client' && <Field label="Client"><Select value={f.target_id} onChange={(e) => setF({ ...f, target_id: e.target.value })} required><option value="">Select…</option>{clients.map((c) => <option key={c.client_id} value={c.client_id}>{c.name} ({c.client_id})</option>)}</Select></Field>}
      {f.scope === 'global' && <div />}
      <Field label="Probability (0–1)" hint="1 = always; 0.3 = 30% of matching calls"><Input type="number" step="0.05" min={0} max={1} value={f.probability} onChange={(e) => setF({ ...f, probability: e.target.value })} /></Field>
      <Field label="Extra latency (ms)"><Input type="number" min={0} value={f.latency_ms} onChange={(e) => setF({ ...f, latency_ms: e.target.value })} /></Field>
      <Field label="Only the first N calls" hint="0 = unlimited (hit counter resets on edit)"><Input type="number" min={0} value={f.max_hits} onChange={(e) => setF({ ...f, max_hits: e.target.value, hits: 0 })} /></Field>
      {eff?.params?.map((p: string) => <Field key={p} label={`param: ${p}`}><Input value={f.params[p] || ''} onChange={(e) => setF({ ...f, params: { ...f.params, [p]: e.target.value } })} /></Field>)}
      <label className="flex items-center gap-2 text-sm sm:col-span-2"><Toggle checked={f.enabled} onChange={(v) => setF({ ...f, enabled: v })} label="Enabled" /> Enabled</label>
      <Button variant="console" type="submit" className="sm:col-span-2">Save scenario</Button>
    </form>
  );
}
