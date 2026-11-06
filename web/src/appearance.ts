// SPDX-License-Identifier: Apache-2.0
import { useEffect } from 'react';
import { useQuery } from '@tanstack/react-query';
import { api } from './api';

const IMPORTED_FAMILY = 'Diffluence Imported';
const NOTES_POLL_MS = 30_000;

export interface ImportedFont {
  name: string;
  format: string;
  version: string;
}

interface AccentColours {
  accent: string;
  hover: string;
  tint: string;
}

/** What the instance looks like, and what anyone may know before logging in. */
export interface Appearance {
  font: ImportedFont | null;
  name: string;
  accent: { light: AccentColours; dark: AccentColours } | null;
  login_welcome: string;
  terms_url: string;
  banner: { text: string; level: string } | null;
  welcome_page: string;
  help_url: string;
  maintenance: { on: boolean; message: string };
  logo: string;
  mail_ready: boolean;
  self_signup: boolean;
}

export const appearanceQuery = {
  queryKey: ['appearance'],
  queryFn: () => api.poll<Appearance>('/appearance'),
};

/** The same answer, kept fresh, so maintenance mode reaches an open screen. */
export const liveAppearanceQuery = { ...appearanceQuery, refetchInterval: NOTES_POLL_MS };

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

const ACCENT_TOKENS = [
  ['--color-accent', 'accent'],
  ['--color-accent-hover', 'hover'],
  ['--color-accent-tint', 'tint'],
] as const;

/** Applies the accent pair of the instance. The server already checked its contrast. */
export function useAccent(): void {
  const accent = useQuery(appearanceQuery).data?.accent;
  useEffect(() => {
    if (!accent) return;
    const root = document.documentElement;
    for (const [token, part] of ACCENT_TOKENS) {
      root.style.setProperty(token, `light-dark(${accent.light[part]}, ${accent.dark[part]})`);
    }
    return () => {
      for (const [token] of ACCENT_TOKENS) root.style.removeProperty(token);
    };
  }, [accent]);
}
