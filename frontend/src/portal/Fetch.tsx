import { useState, type FormEvent } from 'react';
import { portal, request } from '../lib/api';
import { navigate } from '../lib/router';
import { Alert, Badge, Button, Card, Field, Input, Loading, PageHeader, cx, toast, useAsync } from '../components/ui';

export function FetchDocuments() {
  const meta = useAsync(() => request('GET', '/portal-api/issuers'));
  const me = useAsync(() => portal('GET', '/me'));
  const [sel, setSel] = useState<{ issuer: any; doc: any } | null>(null);
  const [params, setParams] = useState<Record<string, string>>({});
  const [err, setErr] = useState('');
  const [busy, setBusy] = useState(false);
  if (meta.loading && !meta.data) return <Loading />;

  const choose = (issuer: any, doc: any) => {
    setSel({ issuer, doc }); setErr('');
    const u = me.data || {};
    const pre: Record<string, string> = {};
    for (const p of doc.parameters) {
      if (p.paramname === 'panno') pre[p.paramname] = u.pan || '';
      else if (p.paramname === 'PANFullName') pre[p.paramname] = u.name || '';
      else if (p.paramname === 'dlno') pre[p.paramname] = u.driving_licence || '';
      else if (p.paramname === 'DOB' && u.dob) pre[p.paramname] = u.dob.split('-').reverse().join('-');
      else pre[p.paramname] = '';
    }
    setParams(pre);
  };

  const submit = async (e: FormEvent) => {
    e.preventDefault(); setBusy(true); setErr('');
    try {
      const d = await portal('POST', '/pull', { orgid: sel!.issuer.orgid, doctype: sel!.doc.doctype, params });
      toast.ok(d.description + ' added to your locker');
      navigate('/documents/' + encodeURIComponent(d.uri));
    } catch (e) { setErr(e instanceof Error ? e.message : String(e)); } finally { setBusy(false); }
  };

  return (
    <div>
      <PageHeader title="Fetch documents from issuers" subtitle="Simulates DigiLocker's Pull Document: the mock issuer matches your details against this persona's synthetic records." />
      <div className="grid gap-6 lg:grid-cols-5">
        <div className="flex flex-col gap-4 lg:col-span-3">
          {meta.data?.issuers.map((is: any) => (
            <Card key={is.orgid} title={is.name} actions={<Badge tone={is.available ? 'green' : 'red'}>{is.available ? 'online' : 'unavailable'}</Badge>}>
              <div className="grid gap-2 sm:grid-cols-2">
                {is.doctypes.map((d: any) => (
                  <button key={d.doctype} onClick={() => choose(is, d)}
                    className={cx('rounded-lg border p-3 text-left transition hover:border-brand-500', sel?.doc.doctype === d.doctype ? 'border-brand-500 bg-brand-50' : 'border-slate-200')}>
                    <div className="text-sm font-semibold">{d.description}</div><div className="font-mono text-xs text-slate-500">{d.doctype}</div>
                  </button>
                ))}
              </div>
            </Card>
          ))}
        </div>
        <Card title={sel ? `Fetch ${sel.doc.description}` : 'Choose a document'} className="h-fit lg:col-span-2">
          {!sel ? <p className="text-sm text-slate-500">Pick a document type on the left.</p> : (
            <form onSubmit={submit} className="flex flex-col gap-4">
              {err && <Alert>{err}</Alert>}
              {sel.doc.parameters.length === 0 && <Alert tone="blue">No details needed — this document is fetched using your linked (synthetic) Aadhaar.</Alert>}
              {sel.doc.parameters.map((p: any) => (
                <Field key={p.paramname} label={p.label} hint={<>param <code>{p.paramname}</code> · e.g. {p.example}</>}>
                  <Input value={params[p.paramname] || ''} onChange={(e) => setParams({ ...params, [p.paramname]: e.target.value })} required />
                </Field>
              ))}
              <p className="text-xs text-slate-500">I consent to share my (synthetic) name, date of birth and identifiers with {sel.issuer.name} to fetch this document.</p>
              <Button type="submit" loading={busy}>Fetch document</Button>
            </form>
          )}
        </Card>
      </div>
    </div>
  );
}
