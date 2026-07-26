import { describe, it, expect } from 'vitest';

import { formatPanelVersion } from '@/lib/panel-version';

describe('formatPanelVersion', () => {
  it('adds a single v prefix to bare semantic versions', () => {
    expect(formatPanelVersion('3.4.0')).toBe('v3.4.0');
    expect(formatPanelVersion('2.6.5')).toBe('v2.6.5');
  });

  it('does not double up the v on already-prefixed tags', () => {
    expect(formatPanelVersion('v3.4.0')).toBe('v3.4.0');
    expect(formatPanelVersion('V3.4.0')).toBe('v3.4.0');
  });

  it('shows dev builds verbatim without a v prefix', () => {
    expect(formatPanelVersion('dev+1a2b3c4d')).toBe('dev+1a2b3c4d');
    expect(formatPanelVersion('dev')).toBe('dev');
  });

  it('returns empty for blank input and leaves unknown markers untouched', () => {
    expect(formatPanelVersion('')).toBe('');
    expect(formatPanelVersion(undefined)).toBe('');
    expect(formatPanelVersion('?')).toBe('?');
  });
});
