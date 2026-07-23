import { useEffect } from 'react';

/** Lock background scroll for the lifetime of the calling component.
 *  Used by modal dialogs so the underlying canvas / list cannot drift
 *  while the dialog is open. Restores the previous overflow value on
 *  unmount, even if it was customized elsewhere. */
export function useBodyScrollLock(): void {
  useEffect(() => {
    const original = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = original;
    };
  }, []);
}
