// SPDX-License-Identifier: Apache-2.0
import { api, ApiError } from './api';

/** Whether the first setup is open. The server answers 404 once an administrator exists. */
export const setupStateQuery = {
  queryKey: ['setup'],
  queryFn: async (): Promise<boolean> => {
    try {
      await api.get('/setup');
      return true;
    } catch (e) {
      if (e instanceof ApiError && e.status === 404) return false;
      throw e;
    }
  },
  retry: false,
};
