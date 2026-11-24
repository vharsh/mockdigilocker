import { createRoot } from 'react-dom/client';
import { useLocation } from './lib/router';
import { Toaster } from './components/ui';
import { PortalApp } from './portal/PortalApp';
import { AdminApp } from './admin/AdminApp';
import { DocsApp } from './docs/DocsApp';

function App() {
  const { path } = useLocation();
  const area = path.startsWith('/admin') ? 'admin' : path.startsWith('/docs') ? 'docs' : 'portal';
  return (
    <>
      {area === 'admin' ? <AdminApp /> : area === 'docs' ? <DocsApp /> : <PortalApp />}
      <Toaster />
    </>
  );
}

createRoot(document.getElementById('root')!).render(<App />);
