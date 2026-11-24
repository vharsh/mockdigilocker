import { request, runtime } from '../lib/api';
import { navigate } from '../lib/router';
import { Badge, Button, Card, Code, useAsync } from '../components/ui';
import { docTypeLabel } from '../lib/format';

export function Landing({ session }: { session: any }) {
  const meta = useAsync(() => request('GET', '/portal-api/meta'));
  const base = runtime.publicBaseUrl;
  return (
    <div className="flex flex-col gap-10">
      <section className="grid items-center gap-10 lg:grid-cols-2">
        <div>
          <Badge tone="red">Sandbox · synthetic data only</Badge>
          <h1 className="mt-4 text-4xl font-bold tracking-tight text-slate-900 sm:text-5xl">Test your DigiLocker integration without real documents.</h1>
          <p className="mt-4 text-lg text-slate-600">
            A self-hostable, DigiLocker-compatible mock. Point your QA/UAT build at this server and run the whole
            authorization, consent and document-retrieval lifecycle with fictional personas.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            {session ? <Button onClick={() => navigate('/dashboard')}>Open my locker</Button> : <Button onClick={() => navigate('/login')}>Sign in with a test persona</Button>}
            <Button variant="secondary" onClick={() => navigate('/register')}>Create test account</Button>
            <a href="/docs" className="inline-flex items-center rounded-lg px-4 py-2 text-sm font-semibold text-teal-700 hover:bg-teal-50">Developer docs →</a>
          </div>
          <p className="mt-6 text-sm text-slate-500">
            Not affiliated with DigiLocker or MeitY. Documents generated here carry the watermark
            <em> “DEMO DOCUMENT – NOT VALID FOR IDENTIFICATION”</em> and have no legal validity.
          </p>
        </div>
        <Card title="Point your app here" actions={<a href="/demo-partner" className="text-xs font-semibold text-brand-600">Try the demo partner →</a>}>
          <p className="mb-3 text-sm text-slate-600">Swap the DigiLocker base URL in your QA config. Paths and payloads follow the Authorized Partner API v1.11.</p>
          <Code>{`DIGILOCKER_AUTHORIZE_URL=${base}/public/oauth2/1/authorize
DIGILOCKER_TOKEN_URL=${base}/public/oauth2/1/token
DIGILOCKER_API_BASE=${base}/public/oauth2
DIGILOCKER_CLIENT_ID=MOCKDEMO01
DIGILOCKER_CLIENT_SECRET=mock-demo-secret-change-me`}</Code>
          <div className="mt-4 grid grid-cols-3 gap-2 text-center text-xs">
            <a href="/admin" className="rounded-lg bg-slate-900 px-2 py-3 font-semibold text-amber-300 hover:bg-slate-800">Developer console</a>
            <a href="/docs/api-reference" className="rounded-lg bg-teal-50 px-2 py-3 font-semibold text-teal-800 hover:bg-teal-100">API reference</a>
            <a href="/.well-known/openid-configuration" className="rounded-lg bg-slate-100 px-2 py-3 font-semibold text-slate-700 hover:bg-slate-200">OIDC discovery</a>
          </div>
        </Card>
      </section>

      <section>
        <h2 className="mb-4 text-lg font-semibold">Mock issuers &amp; document types</h2>
        <div className="grid gap-4 md:grid-cols-3">
          {(meta.data?.issuers || []).map((is: any) => (
            <Card key={is.orgid}>
              <div className="flex items-start justify-between gap-2">
                <div>
                  <div className="font-semibold text-slate-900">{is.name}</div>
                  <div className="font-mono text-xs text-slate-500">{is.issuerid} · orgid {is.orgid}</div>
                </div>
                <Badge tone={is.available ? 'green' : 'red'}>{is.available ? 'online' : 'down'}</Badge>
              </div>
              <ul className="mt-3 flex flex-wrap gap-2">
                {is.doctypes.map((d: any) => <li key={d.doctype}><Badge tone="violet">{d.doctype}</Badge> <span className="text-xs text-slate-600">{docTypeLabel[d.doctype] || d.description}</span></li>)}
              </ul>
            </Card>
          ))}
        </div>
      </section>

      <section className="grid gap-4 md:grid-cols-3">
        {[
          ['OAuth 2.0 + PKCE', 'Authorization code, implicit, refresh and device flows; S256 PKCE; OIDC id_token (RS256) with JWKS.'],
          ['Consent you control', 'Approve, deny, select documents, set validity, revoke later — or automate it with scenarios for headless tests.'],
          ['Failure injection', 'Expired tokens, reused codes, issuer outages, timeouts, tampered documents — per user, per client or globally.'],
        ].map(([t, d]) => <Card key={t}><div className="font-semibold">{t}</div><p className="mt-1 text-sm text-slate-600">{d}</p></Card>)}
      </section>
    </div>
  );
}
