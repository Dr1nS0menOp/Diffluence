// SPDX-License-Identifier: Apache-2.0
import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from './api';

const IMPORTED_FAMILY = 'Diffluence Imported';

export interface ImportedFont {
  name: string;
  format: string;
  version: string;
}

export const appearanceQuery = {
  queryKey: ['appearance'],
  queryFn: () => api.get<{ font: ImportedFont | null }>('/appearance'),
};

/**
 * Applies the font an administrator imported, here and only here. The stack
 * behind it stays, so a font that does not load simply leaves the system font.
 */
export function useImportedFont(): void {
  const version = useQuery(appearanceQuery).data?.font?.version;
  useEffect(() => {
    if (!version) return;
    const root = document.documentElement;
    const address = `url(/api/v1/appearance/font?v=${encodeURIComponent(version)})`;
    const face = new FontFace(IMPORTED_FAMILY, address, { display: 'swap' });
    let replaced = false;
    face
      .load()
      .then(() => {
        if (replaced) return;
        document.fonts.add(face);
        root.style.setProperty('--font-sans', `'${IMPORTED_FAMILY}', var(--font-system)`);
      })
      .catch((error: unknown) => console.warn('The imported font could not be used.', error));
    return () => {
      replaced = true;
      document.fonts.delete(face);
      root.style.removeProperty('--font-sans');
    };
  }, [version]);
}
