// Builds the UI into backend/internal/webui/dist (embedded into the Go binary).
//  - bundles src/main.tsx with esbuild
//  - compiles Tailwind CSS v4 (candidates scanned from src/)
//  - copies docs/*.md, docs/openapi.yaml and the Postman collection
// Works with a normal `npm install`, and offline with globally installed packages.
import fs from 'node:fs';
import path from 'node:path';
import crypto from 'node:crypto';
import { createRequire } from 'node:module';
import { fileURLToPath, pathToFileURL } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(here, '..');
const out = path.join(root, 'backend/internal/webui/dist');
const watch = process.argv.includes('--watch');

// Resolve packages from local node_modules first, then known global dirs.
const searchDirs = [path.join(here, 'node_modules'), '/opt/npm-tools/node_modules',
  path.join(process.env.HOME || '', '.npm-global/lib/node_modules'), '/usr/local/lib/node_modules', '/usr/lib/node_modules'];
function pkgDir(name) {
  for (const d of searchDirs) if (fs.existsSync(path.join(d, name, 'package.json'))) return path.join(d, name);
  throw new Error(`package ${name} not found — run npm install in frontend/`);
}
const req = createRequire(path.join(pkgDir('esbuild'), 'package.json'));
const esbuild = req(pkgDir('esbuild'));
const twDir = pkgDir('tailwindcss');
const { compile } = await import(pathToFileURL(path.join(twDir, 'dist/lib.mjs')).href);

function walk(dir, exts) {
  return fs.readdirSync(dir, { withFileTypes: true }).flatMap((e) => {
    const p = path.join(dir, e.name);
    return e.isDirectory() ? walk(p, exts) : exts.some((x) => p.endsWith(x)) ? [p] : [];
  });
}

async function buildCSS() {
  const input = fs.readFileSync(path.join(here, 'src/styles.css'), 'utf8');
  const loadStylesheet = async (id, base) => {
    let file = id === 'tailwindcss' ? path.join(twDir, 'index.css') : id.startsWith('tailwindcss/') ? path.join(twDir, id.slice(12)) : path.resolve(base, id);
    if (!file.endsWith('.css') && fs.existsSync(file + '.css')) file += '.css';
    return { path: file, base: path.dirname(file), content: fs.readFileSync(file, 'utf8') };
  };
  const compiler = await compile(input, { base: path.join(here, 'src'), loadStylesheet });
  const candidates = new Set();
  for (const f of walk(path.join(here, 'src'), ['.tsx', '.ts'])) {
    for (const m of fs.readFileSync(f, 'utf8').matchAll(/[A-Za-z0-9_\-:/.[\]#%(),!@&>~*]+/g)) candidates.add(m[0]);
  }
  return compiler.build([...candidates]);
}

const hash = (s) => crypto.createHash('sha256').update(s).digest('hex').slice(0, 10);

async function build() {
  const t0 = Date.now();
  fs.rmSync(out, { recursive: true, force: true });
  fs.mkdirSync(path.join(out, 'assets'), { recursive: true });
  const res = await esbuild.build({
    entryPoints: [path.join(here, 'src/main.tsx')], bundle: true, minify: !watch, sourcemap: watch ? 'inline' : false,
    format: 'esm', target: 'es2020', write: false, jsx: 'automatic', loader: { '.svg': 'text' },
    nodePaths: searchDirs, define: { 'process.env.NODE_ENV': JSON.stringify(watch ? 'development' : 'production') },
    alias: { react: pkgDir('react'), 'react-dom': pkgDir('react-dom'), marked: path.join(pkgDir('marked'), 'lib/marked.esm.js') },
    logLevel: 'warning',
  });
  const js = res.outputFiles[0].text;
  const css = await buildCSS();
  const jsName = `assets/app-${hash(js)}.js`, cssName = `assets/app-${hash(css)}.css`;
  fs.writeFileSync(path.join(out, jsName), js);
  fs.writeFileSync(path.join(out, cssName), css);
  const html = fs.readFileSync(path.join(here, 'index.html'), 'utf8').replace('%CSS%', '/' + cssName).replace('%JS%', '/' + jsName);
  fs.writeFileSync(path.join(out, 'index.html'), html);
  fs.copyFileSync(path.join(here, 'favicon.svg'), path.join(out, 'favicon.svg'));
  // docs content
  const docsOut = path.join(out, 'docs-content');
  fs.mkdirSync(docsOut, { recursive: true });
  for (const f of fs.readdirSync(path.join(root, 'docs'))) {
    if (/\.(md|ya?ml|json)$/.test(f)) fs.copyFileSync(path.join(root, 'docs', f), path.join(docsOut, f));
  }
  for (const [src, name] of [['CONTRIBUTING.md', 'contributing.md'], ['SECURITY.md', 'security.md']]) {
    if (fs.existsSync(path.join(root, src))) fs.copyFileSync(path.join(root, src), path.join(docsOut, name));
  }
  const pm = path.join(root, 'postman');
  if (fs.existsSync(pm)) for (const f of fs.readdirSync(pm)) fs.copyFileSync(path.join(pm, f), path.join(docsOut, f));
  console.log(`built UI in ${Date.now() - t0}ms → ${path.relative(root, out)} (js ${(js.length / 1024).toFixed(0)} KB, css ${(css.length / 1024).toFixed(0)} KB)`);
}

await build();
if (watch) {
  let timer;
  for (const d of [path.join(here, 'src'), path.join(root, 'docs')]) {
    fs.watch(d, { recursive: true }, () => { clearTimeout(timer); timer = setTimeout(() => build().catch(console.error), 150); });
  }
  console.log('watching for changes…');
}
