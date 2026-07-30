import type { ReactNode } from 'react';
import { act, renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { afterEach, describe, expect, it, vi } from 'vitest';

import { useAllSettings } from '@/api/queries/useAllSettings';
import { keys } from '@/api/queryKeys';
import { HttpUtil, Msg } from '@/utils';

function makeTestQueryClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe('useAllSettings', () => {
  it('keeps backend-accepted settings editable when the frontend schema is stricter', async () => {
    const webBasePath = 'backend-accepted-without-leading-slash';
    vi.spyOn(HttpUtil, 'post').mockResolvedValue(new Msg(true, '', { webBasePath }));
    const queryClient = makeTestQueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );

    const { result } = renderHook(() => useAllSettings(), { wrapper });

    await waitFor(() => expect(result.current.fetched).toBe(true));
    expect(result.current.allSetting.webBasePath).toBe(webBasePath);
  });

  it('keeps an edited setting when a refetch returns older server data', async () => {
    const values = [
      { webPort: 2053 },
      { webPort: 2054 },
    ];
    let index = 0;
    vi.spyOn(HttpUtil, 'post').mockImplementation(async () => new Msg(true, '', values[index++]));
    const queryClient = makeTestQueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useAllSettings(), { wrapper });

    await waitFor(() => expect(result.current.fetched).toBe(true));
    act(() => result.current.updateSetting({ webPort: 3000 }));
    await queryClient.invalidateQueries({ queryKey: keys.settings.all() });

    await waitFor(() => expect(HttpUtil.post).toHaveBeenCalledTimes(2));
    expect(result.current.allSetting.webPort).toBe(3000);
    expect(result.current.saveDisabled).toBe(false);
  });

  it('hydrates redacted secrets after a successful save', async () => {
    let fetchCount = 0;
    vi.spyOn(HttpUtil, 'post').mockImplementation(async (url) => {
      if (url === '/panel/api/setting/all') {
        fetchCount += 1;
        return new Msg(
          true,
          '',
          fetchCount === 1 ? { hasTwoFactorToken: false } : { hasTwoFactorToken: true, twoFactorToken: '' },
        );
      }
      return new Msg(true, '');
    });
    const queryClient = makeTestQueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useAllSettings(), { wrapper });

    await waitFor(() => expect(result.current.fetched).toBe(true));
    act(() => result.current.updateSetting({ twoFactorToken: 'secret' }));
    await act(async () => {
      await result.current.saveAll();
    });

    await waitFor(() => expect(HttpUtil.post).toHaveBeenCalledTimes(3));
    expect(result.current.allSetting.twoFactorToken).toBe('');
    expect(result.current.allSetting.hasTwoFactorToken).toBe(true);
    expect(result.current.saveDisabled).toBe(true);
  });

  it('establishes a saved baseline for a full-payload security save', async () => {
    vi.spyOn(HttpUtil, 'post').mockImplementation(async (url) => {
      if (url === '/panel/api/setting/all') {
        return new Msg(
          true,
          '',
          { hasTwoFactorToken: false, twoFactorToken: '' },
        );
      }
      return new Msg(true, '');
    });
    const queryClient = makeTestQueryClient();
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    const { result } = renderHook(() => useAllSettings(), { wrapper });

    await waitFor(() => expect(result.current.fetched).toBe(true));
    act(() => result.current.updateSetting({ twoFactorToken: 'secret' }));
    await act(async () => {
      const msg = await result.current.savePayload({
        ...result.current.allSetting,
        twoFactorEnable: false,
        twoFactorToken: '',
      });
      if (msg.success) {
        result.current.updateSetting({
          twoFactorEnable: false,
          twoFactorToken: '',
          hasTwoFactorToken: false,
        });
      }
    });

    await waitFor(() => expect(HttpUtil.post).toHaveBeenCalledTimes(3));
    expect(result.current.allSetting.twoFactorToken).toBe('');
    expect(result.current.allSetting.hasTwoFactorToken).toBe(false);
    expect(result.current.saveDisabled).toBe(true);
  });
});
