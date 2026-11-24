import { useEffect, useMemo, useState } from 'react';
import { marked } from 'marked';
import { Link, useLocation } from '../lib/router';
import { Logo, SandboxBar, Loading, cx } from '../components/ui';
import { runtime } from '../lib/api';

export const sections: { title: string; pages: [string, string][] }[] = [
  { title: 'Start here', pages: [['getting-started', 'Getting started'], ['installation', 'Installation & Docker'], ['configuration', 'Configuration reference']] },
  { title: 'Integrate', pages: [['integration-guide', 'Integration guide'], ['flows', 'Auth, consent & retrieval flows'], ['api-reference', 'API reference'], ['compatibility', 'Compatibility & API profiles'], ['webhooks', 'Webhooks']] },
  { title: 'Test', pages: [['personas', 'Test personas'], ['scenarios', 'Scenario simulator'], ['console', 'Developer console & inspector'], ['troubleshooting', 'Troubleshooting']] },
  { title: 'Project', pages: [['architecture', 'Architecture'], ['contributing', 'Contributing'], ['security', 'Security policy']] },
];

marked.setOptions({ gfm: true });

export function DocsApp() {
  const { path } = useLocation();
  const slug = path.replace(/^\/docs\/?/, '').replace(/\/$/, '') || 'getting-started';
  const [md, setMd] = useState<string | null>(null);
  const [err, setErr] = useState('');
  useEffect(() => {
    setMd(null); setErr('');
    fetch(runtime.apiBase + '/docs-content/' + slug + '.md').then((r) => (r.ok ? r.text() : Promise.reject(new Error('Page not found')))).then(setMd).catch((e) => setErr(e.message));
  }, [slug]);
  const html = useMemo(() => {
    if (!md) return '';
    const base = runtime.publicBaseUrl;
    return marked.parse(md.replaceAll('http://localhost:8080', base).replace(/\]\(\.\/([a-z-]+)\.md(#[^)]*)?\)/g, '](/docs/$1$2)')) as string;
  }, [md]);
  // client-side links inside the rendered markdown
  useEffect(() => {
    const el = document.getElementById('doc-body');
    if (!el) return;
    el.querySelectorAll('h2, h3').forEach((h) => { if (!h.id) h.id = (h.textContent || '').toLowerCase().replace(/[^a-z0-9]+/g, '-'); });
  }, [html]);
  const toc = useMemo(() => [...(md || '').matchAll(/^## (.+)$/gm)].map((m) => m[1]), [md]);

  return (
    <div className="min-h-screen bg-white">
      <SandboxBar variant="docs" />
      <header className="sticky top-0 z-30 border-b border-slate-200 bg-white/95 backdrop-blur">
        <div className="mx-auto flex h-14 max-w-7xl items-center justify-between px-4">
          <Link to="/docs"><Logo sub="Developer documentation" /></Link>
          <nav className="flex items-center gap-4 text-sm font-medium text-slate-600">
            <a href="/docs-content/openapi.yaml" className="hover:text-teal-700">OpenAPI</a>
            <a href="/docs-content/mock-digilocker.postman_collection.json" download className="hidden hover:text-teal-700 sm:inline">Postman</a>
            <a href="/demo-partner" className="hidden hover:text-teal-700 sm:inline">Demo partner</a>
            <a href="/admin" className="hover:text-teal-700">Console</a>
            <a href="/" className="hover:text-teal-700">Portal</a>
          </nav>
        </div>
      </header>
      <div className="mx-auto flex max-w-7xl gap-8 px-4">
        <aside className="sticky top-14 hidden h-[calc(100vh-3.5rem)] w-60 shrink-0 overflow-y-auto py-8 md:block">
          {sections.map((s) => (
            <div key={s.title} className="mb-6">
              <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-400">{s.title}</div>
              {s.pages.map(([p, t]) => (
                <Link key={p} to={'/docs/' + p} className={cx('block rounded-md px-2 py-1.5 text-sm', p === slug ? 'bg-teal-50 font-semibold text-teal-800' : 'text-slate-600 hover:text-slate-900')}>{t}</Link>
              ))}
            </div>
          ))}
        </aside>
        <main className="min-w-0 flex-1 py-8">
          <select className="mb-6 w-full rounded-lg border border-slate-300 px-3 py-2 text-sm md:hidden" value={slug} onChange={(e) => (location.href = '/docs/' + e.target.value)} aria-label="Docs page">
            {sections.flatMap((s) => s.pages).map(([p, t]) => <option key={p} value={p}>{t}</option>)}
          </select>
          {err ? <p className="text-slate-600">{err}. <Link to="/docs" className="text-teal-700 underline">Back to docs home</Link></p> : md === null ? <Loading /> :
            <article id="doc-body" className="prose-docs max-w-3xl" dangerouslySetInnerHTML={{ __html: html }} onClick={(e) => {
              const a = (e.target as HTMLElement).closest('a');
              if (a && a.getAttribute('href')?.startsWith('/docs/')) { e.preventDefault(); history.pushState(null, '', a.getAttribute('href')!); dispatchEvent(new PopStateEvent('popstate')); window.scrollTo(0, 0); }
            }} />}
        </main>
        {toc.length > 2 && (
          <aside className="sticky top-14 hidden h-[calc(100vh-3.5rem)] w-52 shrink-0 overflow-y-auto py-8 xl:block">
            <div className="mb-2 text-xs font-semibold uppercase tracking-wider text-slate-400">On this page</div>
            {toc.map((t) => <a key={t} href={'#' + t.toLowerCase().replace(/[^a-z0-9]+/g, '-')} className="block py-1 text-xs text-slate-600 hover:text-teal-700">{t}</a>)}
          </aside>
        )}
      </div>
    </div>
  );
}
