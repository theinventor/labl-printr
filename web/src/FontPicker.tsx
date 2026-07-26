import { useEffect, useState } from 'react';
import { api, type FontFace } from './api';

// Inject an @font-face per bundled TTF once, so each picker chip can preview
// in its own handwriting. The font files are served by the labl-printr server.
let injected = false;
function injectFontFaces(faces: FontFace[]) {
  if (injected) return;
  injected = true;
  const css = faces
    .filter((f) => f.style === 'handwriting')
    .map(
      (f) =>
        `@font-face{font-family:'labl-${f.id}';src:url('/api/fonts/${f.id}.ttf') format('truetype');font-display:swap;}`,
    )
    .join('\n');
  const el = document.createElement('style');
  el.textContent = css;
  document.head.appendChild(el);
}

export function FontPicker({ value, onChange }: { value: string; onChange: (id: string) => void }) {
  const [faces, setFaces] = useState<FontFace[]>([]);

  useEffect(() => {
    api.fonts().then((list) => {
      setFaces(list);
      injectFontFaces(list);
    });
  }, []);

  if (faces.length === 0) return null;

  return (
    <div className="flex flex-wrap gap-1.5">
      {faces.map((f) => {
        const active = (value || 'system') === f.id;
        return (
          <button
            key={f.id}
            type="button"
            onClick={() => onChange(f.id)}
            style={f.style === 'handwriting' ? { fontFamily: `'labl-${f.id}'` } : undefined}
            className={`rounded-lg border px-3 py-2 text-[15px] leading-none transition-colors ${
              active
                ? 'border-accent/60 bg-accent/10 text-fg'
                : 'border-edge bg-panel-2 text-fg-dim hover:border-edge-2 hover:text-fg'
            }`}
            title={f.name}
          >
            {f.style === 'system' ? 'Aa Default' : f.name}
          </button>
        );
      })}
    </div>
  );
}
