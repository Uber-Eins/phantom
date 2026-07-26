import { screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { describe, expect, it } from 'vitest';

import AppSidebar from '@/layouts/AppSidebar';
import { renderWithProviders } from './test-utils';

describe('AppSidebar', () => {
  it('shows only the local single-machine pages and settings sections', () => {
    renderWithProviders(
      <MemoryRouter initialEntries={['/settings#general']}>
        <AppSidebar />
      </MemoryRouter>,
    );

    for (const item of ['Overview', 'Inbounds', 'Clients', 'Outbounds', 'Routing', 'Panel Settings', 'Xray Configs']) {
      expect(screen.getAllByText(item).length).toBeGreaterThan(0);
    }
    for (const removed of [
      'Nodes',
      'Groups',
      'Hosts',
      'API Docs',
      'Telegram',
      'Email',
      'Subscription',
    ]) {
      expect(screen.queryByText(removed)).toBeNull();
    }

    expect(screen.getAllByText('General').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Authentication').length).toBeGreaterThan(0);
  });
});
