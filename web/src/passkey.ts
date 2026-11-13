// SPDX-License-Identifier: Apache-2.0
import { api } from './api';

/**
 * Proves a passkey to the server, for a login or a sensitive action. `step` goes along
 * with both calls, such as the token of a password that was right (spec 069).
 */
export async function provePasskey(route: string, step: Record<string, string> = {}): Promise<void> {
  const begin = await api.post<{
    options: { publicKey: PublicKeyCredentialRequestOptionsJSON };
    token: string;
  }>(`${route}/begin`, step);
  const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(begin.options.publicKey);
  const credential = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
  if (!credential) throw new Error('no credential');
  await api.post(`${route}/finish`, { ...step, token: begin.token, response: credential.toJSON() });
}
