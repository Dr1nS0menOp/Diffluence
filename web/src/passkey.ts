// SPDX-License-Identifier: Apache-2.0
import { api } from './api';

/** Proves a passkey to the server, for a login or a sensitive action. */
export async function provePasskey(route: string): Promise<void> {
  const begin = await api.post<{
    options: { publicKey: PublicKeyCredentialRequestOptionsJSON };
    token: string;
  }>(`${route}/begin`);
  const publicKey = PublicKeyCredential.parseRequestOptionsFromJSON(begin.options.publicKey);
  const credential = (await navigator.credentials.get({ publicKey })) as PublicKeyCredential | null;
  if (!credential) throw new Error('no credential');
  await api.post(`${route}/finish`, { token: begin.token, response: credential.toJSON() });
}
