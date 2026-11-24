import { useEffect, useState, type FormEvent } from 'react';
import { admin } from '../lib/api';
import { Link, navigate } from '../lib/router';
import { Alert, Badge, Button, Card, Field, Input, KV, Loading, Modal, PageHeader, Select, Toggle, toast, useAsync } from '../components/ui';
import { docTypeLabel, fmtDate, genderLabel, statusTone } from '../lib/format';
import { useCanWrite } from './AdminApp';

export function Personas() {
  const [q, setQ] = useState('');
  const { data, error, loading, reload } = useAsync(() => admin('GET', '/users?q=' + encodeURIComponent(q)), [q]);
  const [creating, setCreating] = useState(false);
  const canWrite = useCanWrite();
  return (
    <div>
      <PageHeader title="Test personas" subtitle="Synthetic DigiLocker accounts. Predefined personas cover common QA cases; create more for your own scenarios."
        actions={canWrite && <Button variant="console" onClick={() => setCreating(true)}>+ New persona</Button>} />
      <Input placeholder="Search name, mobile, e-mail, persona…" value={q} onChange={(e) => setQ(e.target.value)} className="mb-4 max-w-md" aria-label="Search users" />
      {error && <Alert>{error.message}</Alert>}
      {loading && !data ? <Loading /> : (
        <div className="overflow-x-auto rounded-xl border border-slate-200 bg-white shadow-sm">
          <table className="w-full text-sm">
            <thead className="bg-slate-50 text-left text-xs uppercase tracking-wide text-slate-500">
              <tr><th className="px-4 py-3">User</th><th className="px-4 py-3">Login</th><th className="px-4 py-3">Persona</th><th className="px-4 py-3">Docs</th><th className="px-4 py-3">e-Aadhaar</th><th className="px-4 py-3">Status</th></tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {data?.items.map((u: any) => (
                <tr key={u.id} className="cursor-pointer hover:bg-amber-50/50" onClick={() => navigate('/admin/personas/' + u.id)}>
                  <td className="px-4 py-3"><div className="font-semibold text-slate-900">{u.name}</div><div className="max-w-xs truncate text-xs text-slate-500">{u.description || u.id}</div></td>
                  <td className="px-4 py-3 font-mono text-xs">{u.mobile}<br />{u.email}</td>
                  <td className="px-4 py-3">{u.persona ? <Badge tone="violet">{u.persona}</Badge> : <span className="text-xs text-slate-400">custom</span>}</td>
                  <td className="px-4 py-3">{u.document_count}</td>
                  <td className="px-4 py-3">{u.eaadhaar ? 'Y' : 'N'}</td>
                  <td className="px-4 py-3">{u.disabled ? <Badge tone="red">disabled</Badge> : <Badge tone="green">active</Badge>}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
      <Modal open={creating} onClose={() => setCreating(false)} title="New synthetic persona" wide>
        <UserForm onSaved={(u) => { setCreating(false); reload(); navigate('/admin/personas/' + u.id); }} />
      </Modal>
    </div>
  );
}

function UserForm({ user, onSaved }: { user?: any; onSaved: (u: any) => void }) {
  const [f, setF] = useState<any>(() => user ? { ...user, address: { ...user.address }, pin: '' } : {
    name: '', dob: '1990-01-01', gender: 'F', mobile: '', email: '', username: '', pin: '123456', description: '', eaadhaar: true,
    aadhaar_last4: '', pan: '', driving_licence: '', father_name: '', address: { state: 'Karnataka', district: 'Mockpur', pincode: '560999' }, documents: ['ADHAR', 'PANCR'],
  });
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  const set = (k: string) => (e: any) => setF({ ...f, [k]: e.target.value });
  const submit = async (e: FormEvent) => {
    e.preventDefault(); setBusy(true); setErr('');
    try {
      const body = { ...f }; if (!body.pin) delete body.pin;
      const u = user ? await admin('PUT', '/users/' + user.id, body) : await admin('POST', '/users', body);
      toast.ok(user ? 'Persona updated' : 'Persona created');
      if (u.warnings?.length) toast.err(u.warnings.join('; '));
      onSaved(u);
    } catch (e) { setErr(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };
  return (
    <form onSubmit={submit} className="grid gap-4 sm:grid-cols-3">
      {err && <Alert className="sm:col-span-3">{err}</Alert>}
      <Field label="Full name" className="sm:col-span-2"><Input value={f.name} onChange={set('name')} required /></Field>
      <Field label="Username"><Input value={f.username} onChange={set('username')} placeholder="auto" /></Field>
      <Field label="Date of birth"><Input type="date" value={f.dob} onChange={set('dob')} required /></Field>
      <Field label="Gender"><Select value={f.gender} onChange={set('gender')}><option value="F">Female</option><option value="M">Male</option><option value="T">Transgender</option></Select></Field>
      <Field label={user ? 'New PIN (blank = keep)' : 'Security PIN'}><Input value={f.pin} onChange={set('pin')} pattern="\d{6}" placeholder="123456" /></Field>
      <Field label="Mobile (10 digits)"><Input value={f.mobile} onChange={set('mobile')} pattern="\d{10}" required placeholder="99999xxxxx" /></Field>
      <Field label="E-mail"><Input type="email" value={f.email} onChange={set('email')} placeholder="x@sandbox.test" /></Field>
      <Field label="Father's name"><Input value={f.father_name} onChange={set('father_name')} /></Field>
      <Field label="Synthetic Aadhaar last 4" hint="UID becomes 00000000xxxx"><Input value={f.aadhaar_last4} onChange={set('aadhaar_last4')} pattern="\d{4}" /></Field>
      <Field label="PAN (synthetic)" hint="e.g. ZZZPA1234A"><Input value={f.pan} onChange={set('pan')} /></Field>
      <Field label="Driving licence (synthetic)" hint="RTO code 00, e.g. KA0020200000099"><Input value={f.driving_licence} onChange={set('driving_licence')} /></Field>
      <Field label="Description" className="sm:col-span-3"><Input value={f.description} onChange={set('description')} placeholder="What this persona is for" /></Field>
      <Field label="State"><Input value={f.address?.state || ''} onChange={(e) => setF({ ...f, address: { ...f.address, state: e.target.value } })} /></Field>
      <Field label="District"><Input value={f.address?.district || ''} onChange={(e) => setF({ ...f, address: { ...f.address, district: e.target.value } })} /></Field>
      <Field label="PIN code"><Input value={f.address?.pincode || ''} onChange={(e) => setF({ ...f, address: { ...f.address, pincode: e.target.value } })} /></Field>
      <div className="flex items-center gap-6 sm:col-span-3">
        <label className="flex items-center gap-2 text-sm"><Toggle checked={!!f.eaadhaar} onChange={(v) => setF({ ...f, eaadhaar: v })} label="e-Aadhaar" /> e-Aadhaar linked</label>
        {user && <label className="flex items-center gap-2 text-sm"><Toggle checked={!!f.disabled} onChange={(v) => setF({ ...f, disabled: v })} label="Disabled" /> Account disabled</label>}
      </div>
      {!user && (
        <div className="flex flex-wrap gap-4 sm:col-span-3">
          <span className="text-xs font-semibold text-slate-600">Generate documents:</span>
          {['ADHAR', 'PANCR', 'DRVLC'].map((d) => (
            <label key={d} className="flex items-center gap-1.5 text-sm"><input type="checkbox" checked={f.documents.includes(d)}
              onChange={(e) => setF({ ...f, documents: e.target.checked ? [...f.documents, d] : f.documents.filter((x: string) => x !== d) })} />{docTypeLabel[d]}</label>
          ))}
        </div>
      )}
      <div className="sm:col-span-3"><Button variant="console" type="submit" loading={busy}>{user ? 'Save persona' : 'Create persona'}</Button></div>
    </form>
  );
}

export function PersonaDetail({ id }: { id: string }) {
  const { data: u, error, loading, reload } = useAsync(() => admin('GET', '/users/' + id), [id]);
  const [edit, setEdit] = useState(false);
  const [assign, setAssign] = useState(false);
  const [docEdit, setDocEdit] = useState<any>(null);
  const canWrite = useCanWrite();
  if (loading && !u) return <Loading />;
  if (error) return <Alert>{error.message}</Alert>;
  const act = async (label: string, fn: () => Promise<any>) => {
    try { const r = await fn(); toast.ok(label + (r?.removed_sessions_and_tokens !== undefined ? ` (${r.removed_sessions_and_tokens} removed)` : '')); reload(); } catch (e) { toast.err(e); }
  };
  return (
    <div>
      <Link to="/admin/personas" className="text-sm font-medium text-amber-700">← All personas</Link>
      <div className="mt-3"><PageHeader title={<span className="flex items-center gap-3">{u.name}{u.disabled ? <Badge tone="red">disabled</Badge> : <Badge tone="green">active</Badge>}</span>}
        subtitle={<code>{u.id}</code>}
        actions={canWrite && <>
          <Button variant="secondary" onClick={() => act(u.disabled ? 'Enabled' : 'Disabled', () => admin('PUT', '/users/' + id, { disabled: !u.disabled }))}>{u.disabled ? 'Enable' : 'Disable'} account</Button>
          <Button variant="secondary" onClick={() => act('Sessions reset', () => admin('POST', `/users/${id}/reset-sessions`))}>Reset sessions</Button>
          {u.persona && <Button variant="secondary" onClick={() => act('Documents restored', () => admin('POST', `/users/${id}/reset-documents`))}>Restore persona docs</Button>}
          <Button variant="console" onClick={() => setEdit(true)}>Edit</Button>
          <Button variant="danger" onClick={() => confirm('Delete this persona and its documents?') && act('Deleted', () => admin('DELETE', '/users/' + id)).then(() => navigate('/admin/personas'))}>Delete</Button>
        </>} /></div>
      <div className="grid gap-6 lg:grid-cols-3">
        <Card title="Attributes">
          <KV className="sm:grid-cols-1" items={[['Persona', u.persona || 'custom'], ['Description', u.description], ['DOB', fmtDate(u.dob)], ['Gender', genderLabel(u.gender)],
            ['Mobile / e-mail', <>{u.mobile}<br />{u.email}</>], ['Username', u.username], ['e-Aadhaar', u.eaadhaar ? 'linked' : 'not linked'],
            ['Synthetic UID', u.synthetic_uid], ['PAN', u.pan], ['Driving licence', u.driving_licence], ['Reference key', <code className="break-all text-[11px]">{u.reference_key}</code>]]} />
        </Card>
        <div className="flex flex-col gap-6 lg:col-span-2">
          <Card title={`Documents (${u.documents.length})`} actions={canWrite && <Button size="sm" variant="console" onClick={() => setAssign(true)}>+ Assign document</Button>}>
            {u.documents.length === 0 ? <p className="text-sm text-slate-500">No documents.</p> : (
              <table className="w-full text-sm">
                <thead className="text-left text-xs uppercase text-slate-500"><tr><th className="py-2">Document</th><th>Number</th><th>Valid to</th><th>Status</th><th /></tr></thead>
                <tbody className="divide-y divide-slate-100">
                  {u.documents.map((d: any) => (
                    <tr key={d.uri}>
                      <td className="py-2"><div className="font-medium">{d.description}</div><code className="text-[11px] text-slate-500">{d.uri}</code></td>
                      <td className="font-mono text-xs">{d.doc_number}</td><td className="text-xs">{fmtDate(d.valid_to)}</td>
                      <td><Badge tone={statusTone(d.status)}>{d.status}</Badge></td>
                      <td className="text-right">{canWrite && <Button size="sm" variant="ghost" onClick={() => setDocEdit(d)}>Edit</Button>}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </Card>
          <Card title="Consents">
            {u.consents.length === 0 ? <p className="text-sm text-slate-500">No consents.</p> : (
              <ul className="divide-y divide-slate-100 text-sm">{u.consents.map((c: any) => (
                <li key={c.id} className="flex items-center justify-between py-2"><span>{c.client_name} <span className="text-xs text-slate-500">· {c.granted_uris.length} docs · {fmtDate(c.created_at, true)}</span></span><Badge tone={statusTone(c.status)}>{c.status}</Badge></li>
              ))}</ul>
            )}
          </Card>
        </div>
      </div>
      <Modal open={edit} onClose={() => setEdit(false)} title="Edit persona" wide><UserForm user={u} onSaved={() => { setEdit(false); reload(); }} /></Modal>
      <Modal open={assign} onClose={() => setAssign(false)} title="Assign a document"><AssignDoc userId={id} onDone={() => { setAssign(false); reload(); }} /></Modal>
      <Modal open={!!docEdit} onClose={() => setDocEdit(null)} title="Edit document">{docEdit && <DocEditor doc={docEdit} onDone={() => { setDocEdit(null); reload(); }} />}</Modal>
    </div>
  );
}

function AssignDoc({ userId, onDone }: { userId: string; onDone: () => void }) {
  const [f, setF] = useState({ doctype: 'PANCR', status: 'valid', issue_date: '', valid_to: '', doc_number: '' });
  const [err, setErr] = useState('');
  const submit = async (e: FormEvent) => {
    e.preventDefault(); setErr('');
    try { await admin('POST', `/users/${userId}/documents`, f); toast.ok('Document issued'); onDone(); } catch (e) { setErr(e instanceof Error ? e.message : String(e)); }
  };
  return (
    <form onSubmit={submit} className="grid gap-4 sm:grid-cols-2">
      {err && <Alert className="sm:col-span-2">{err}</Alert>}
      <Field label="Document type"><Select value={f.doctype} onChange={(e) => setF({ ...f, doctype: e.target.value })}>{Object.entries(docTypeLabel).map(([k, v]) => <option key={k} value={k}>{v} ({k})</option>)}</Select></Field>
      <Field label="Status"><Select value={f.status} onChange={(e) => setF({ ...f, status: e.target.value })}>{['valid', 'invalid', 'revoked', 'unavailable'].map((s) => <option key={s}>{s}</option>)}</Select></Field>
      <Field label="Issue date"><Input type="date" value={f.issue_date} onChange={(e) => setF({ ...f, issue_date: e.target.value })} /></Field>
      <Field label="Valid to" hint="past date ⇒ expired"><Input type="date" value={f.valid_to} onChange={(e) => setF({ ...f, valid_to: e.target.value })} /></Field>
      <Field label="Document number (optional)" hint="defaults to the persona's identifier" className="sm:col-span-2"><Input value={f.doc_number} onChange={(e) => setF({ ...f, doc_number: e.target.value })} /></Field>
      <Button variant="console" type="submit" className="sm:col-span-2">Issue document</Button>
    </form>
  );
}

function DocEditor({ doc, onDone }: { doc: any; onDone: () => void }) {
  const [f, setF] = useState({ status: doc.stored_status, issue_date: doc.issue_date || '', valid_from: doc.valid_from || '', valid_to: doc.valid_to || '', doc_number: doc.doc_number, xml: doc.mime.includes('application/xml') });
  useEffect(() => setF((x) => x), [doc]);
  const save = async () => {
    try {
      await admin('PUT', '/documents/' + encodeURIComponent(doc.uri), { status: f.status, issue_date: f.issue_date, valid_from: f.valid_from, valid_to: f.valid_to, doc_number: f.doc_number,
        mime: f.xml ? ['application/pdf', 'application/xml'] : ['application/pdf'] });
      toast.ok('Document updated'); onDone();
    } catch (e) { toast.err(e); }
  };
  const del = async () => { if (!confirm('Remove this document from the locker?')) return; try { await admin('DELETE', '/documents/' + encodeURIComponent(doc.uri)); onDone(); } catch (e) { toast.err(e); } };
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <code className="break-all text-xs text-slate-500 sm:col-span-2">{doc.uri}</code>
      <Field label="Status" hint="unavailable ⇒ API returns 503"><Select value={f.status} onChange={(e) => setF({ ...f, status: e.target.value })}>{['valid', 'expired', 'invalid', 'revoked', 'unavailable'].map((s) => <option key={s}>{s}</option>)}</Select></Field>
      <Field label="Document number"><Input value={f.doc_number} onChange={(e) => setF({ ...f, doc_number: e.target.value })} /></Field>
      <Field label="Issue date"><Input type="date" value={f.issue_date} onChange={(e) => setF({ ...f, issue_date: e.target.value })} /></Field>
      <Field label="Valid from"><Input type="date" value={f.valid_from} onChange={(e) => setF({ ...f, valid_from: e.target.value })} /></Field>
      <Field label="Valid to"><Input type="date" value={f.valid_to} onChange={(e) => setF({ ...f, valid_to: e.target.value })} /></Field>
      <label className="flex items-center gap-2 self-end text-sm"><Toggle checked={f.xml} onChange={(v) => setF({ ...f, xml: v })} label="XML" /> XML available</label>
      <div className="flex justify-between sm:col-span-2"><Button variant="danger" onClick={del}>Remove</Button><Button variant="console" onClick={save}>Save</Button></div>
    </div>
  );
}
