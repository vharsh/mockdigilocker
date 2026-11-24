import { useEffect, useState } from 'react';
import { portal, portalStore, openBlob, runtime } from '../lib/api';
import { Link, navigate } from '../lib/router';
import { Alert, Badge, Button, Card, Empty, Input, KV, Loading, PageHeader, useAsync, toast } from '../components/ui';
import { fmtDate, statusTone } from '../lib/format';
import { DocIcon } from './DocIcon';

export function Documents() {
  const [q, setQ] = useState('');
  const [debounced, setDebounced] = useState('');
  useEffect(() => { const t = setTimeout(() => setDebounced(q), 250); return () => clearTimeout(t); }, [q]);
  const { data, error, loading } = useAsync(() => portal('GET', '/documents?q=' + encodeURIComponent(debounced)), [debounced]);
  return (
    <div>
      <PageHeader title="Issued documents" subtitle="Synthetic documents issued to this sandbox account by mock issuers."
        actions={<Button variant="secondary" onClick={() => navigate('/fetch')}>+ Fetch from issuer</Button>} />
      <Input placeholder="Search by name, type (PANCR), number, issuer or URI…" value={q} onChange={(e) => setQ(e.target.value)} className="mb-5 max-w-lg" aria-label="Search documents" />
      {error && <Alert>{error.message}</Alert>}
      {loading && !data ? <Loading /> : data?.items.length === 0 ? (
        <Card><Empty title={debounced ? 'No documents match your search' : 'No issued documents yet'}>{!debounced && <>Use <Link to="/fetch" className="text-brand-600 underline">Fetch from issuer</Link> to pull a document, or assign one from the developer console.</>}</Empty></Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2">
          {data?.items.map((d: any) => (
            <Link key={d.uri} to={'/documents/' + encodeURIComponent(d.uri)} className="flex gap-4 rounded-xl border border-slate-200 bg-white p-4 shadow-sm transition hover:border-brand-500">
              <DocIcon type={d.doctype} />
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2"><span className="font-semibold text-slate-900">{d.description}</span><Badge tone={statusTone(d.status)}>{d.status}</Badge></div>
                <div className="text-sm text-slate-600">{d.issuer.name}</div>
                <div className="mt-1 font-mono text-xs text-slate-500">{d.doc_number}</div>
                <div className="mt-1 text-xs text-slate-500">Issued {fmtDate(d.issue_date)}{d.valid_to && <> · valid till {fmtDate(d.valid_to)}</>}</div>
              </div>
            </Link>
          ))}
        </div>
      )}
    </div>
  );
}

export function DocumentDetail({ uri }: { uri: string }) {
  const { data: d, error, loading } = useAsync(() => portal('GET', '/documents/' + encodeURIComponent(uri)), [uri]);
  const [pdfUrl, setPdfUrl] = useState<string>('');
  useEffect(() => {
    let url = '';
    fetch(runtime.apiBase + '/portal-api/documents/' + encodeURIComponent(uri) + '/pdf', { headers: { Authorization: 'Bearer ' + portalStore.get()?.access_token } })
      .then((r) => (r.ok ? r.blob() : null)).then((b) => { if (b) { url = URL.createObjectURL(b); setPdfUrl(url); } });
    return () => { if (url) URL.revokeObjectURL(url); };
  }, [uri]);
  if (loading && !d) return <Loading />;
  if (error) return <Alert>{error.message}</Alert>;
  const tok = portalStore.get()?.access_token;
  const dl = (fmt: 'pdf' | 'xml') => openBlob(`/portal-api/documents/${encodeURIComponent(uri)}/${fmt}${fmt === 'pdf' ? '?download=1' : ''}`, tok, `${d.doctype}-DEMO.${fmt}`).catch(toast.err);
  const v = d.verification;
  return (
    <div>
      <Link to="/documents" className="text-sm font-medium text-brand-600">← All documents</Link>
      <div className="mt-3"><PageHeader title={<span className="flex items-center gap-3"><DocIcon type={d.doctype} />{d.description}</span>}
        subtitle={<code className="break-all">{d.uri}</code>}
        actions={<><Button variant="secondary" onClick={() => dl('xml')} disabled={!d.mime?.includes('application/xml')}>Download XML</Button><Button onClick={() => dl('pdf')}>Download PDF</Button></>} /></div>
      <div className="grid gap-6 lg:grid-cols-5">
        <div className="flex flex-col gap-6 lg:col-span-2">
          <Card title="Verification status">
            <div className="flex items-center gap-3">
              <div className={`grid h-10 w-10 place-items-center rounded-full text-lg ${v.verified ? 'bg-emerald-100 text-emerald-700' : 'bg-rose-100 text-rose-700'}`}>{v.verified ? '✓' : '!'}</div>
              <div><Badge tone={statusTone(v.status)}>{v.status}</Badge><p className="mt-1 text-sm text-slate-600">{v.message}</p></div>
            </div>
            <p className="mt-3 text-xs text-slate-500">Checked {fmtDate(v.checked_at, true)}. {v.watermark}.</p>
          </Card>
          <Card title="Metadata">
            <KV items={[['Document type', <code>{d.doctype}</code>], ['Document number', d.doc_number], ['Issue date', fmtDate(d.issue_date)], ['Valid from', fmtDate(d.valid_from)],
              ['Valid until', fmtDate(d.valid_to)], ['Formats', d.mime?.join(', ')], ['Added to locker', fmtDate(d.created_at, true)], ['Last updated', fmtDate(d.updated_at, true)]]} />
          </Card>
          <Card title="Issuer">
            <KV items={[['Name', d.issuer.name], ['Issuer ID', <code>{d.issuer.issuerid}</code>], ['Org ID', d.issuer.orgid], ['Category', d.issuer.category],
              ['Repository', d.issuer.available === false ? <Badge tone="red">unavailable</Badge> : <Badge tone="green">online</Badge>]]} />
          </Card>
          <Card title="Document details">
            <KV className="sm:grid-cols-1" items={(d.labels as [string, string][]).filter(([k]) => d.fields?.[k]).map(([k, l]) => [l, k === 'dob' ? fmtDate(d.fields[k]) : d.fields[k]])} />
          </Card>
          <Card title="Shared with">
            {d.shared_with?.length ? (
              <ul className="divide-y divide-slate-100 text-sm">
                {d.shared_with.map((s: any) => <li key={s.consent_id} className="flex items-center justify-between py-2"><span>{s.client_name}</span><Badge tone={statusTone(s.status)}>{s.status}</Badge></li>)}
              </ul>
            ) : <p className="text-sm text-slate-500">Not shared with any application yet.</p>}
          </Card>
        </div>
        <Card title="Preview (PDF)" className="lg:col-span-3">
          {pdfUrl ? <iframe title="Document preview" src={pdfUrl} className="h-[760px] w-full rounded-lg border border-slate-200" /> : <Loading />}
        </Card>
      </div>
    </div>
  );
}
