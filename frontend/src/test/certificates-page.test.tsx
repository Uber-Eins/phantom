import { fireEvent, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';

import { QueryProvider } from '@/api/QueryProvider';
import CertificateConfigModal from '@/pages/certificates/CertificateConfigModal';
import CertificateFormModal from '@/pages/certificates/CertificateFormModal';
import CertificateList from '@/pages/certificates/CertificateList';
import { CertificateIssueRequestSchema } from '@/schemas/certificate';
import { setDatepicker } from '@/hooks/useDatepicker';
import { listSelectOptions, renderWithProviders } from './test-utils';

function renderCertificateForm() {
  renderWithProviders(
    <QueryProvider>
      <CertificateFormModal
        open
        defaultEmail="default@example.com"
        certificate={null}
        onClose={() => {}}
      />
    </QueryProvider>,
  );
}

describe('certificate management', () => {
  it('shows the certificate table and management actions', () => {
    setDatepicker('gregorian');
    const onAdd = vi.fn();
    const onConfig = vi.fn();
    const onEdit = vi.fn();
    const onDelete = vi.fn();
    const { container } = renderWithProviders(
      <CertificateList
        certificates={[{
          id: 1,
          remark: 'Production',
          addMethod: 'acme',
          ca: 'zerossl',
          validationMethod: 'cloudflare',
          identifiers: 'renewal.example.com',
          certificateIdentifiers: 'example.com\n*.example.com',
          email: 'admin@example.com',
          keyType: 'EC256',
          certificateType: 'domain',
          certificateFile: '/managed/fullchain.pem',
          keyFile: '/managed/privkey.pem',
          issuedAt: '2026-01-01T00:00:00Z',
          expiresAt: '2026-04-01T00:00:00Z',
        }]}
        isMobile={false}
        onAdd={onAdd}
        onConfig={onConfig}
        onEdit={onEdit}
        onDelete={onDelete}
      />,
    );

    const headers = Array.from(container.querySelectorAll('th'))
      .map((header) => header.textContent?.trim());
    expect(headers).toContain('ID');
    expect(headers).toContain('Issued at');
    expect(headers).toContain('Expires at');
    expect(screen.getByText('example.com, *.example.com')).toBeTruthy();
    expect(screen.getByText('ZeroSSL')).toBeTruthy();
    expect(screen.getByText('Cloudflare')).toBeTruthy();

    fireEvent.click(container.querySelector('button[aria-label="Add certificate"]')!);
    fireEvent.click(container.querySelector('button[aria-label="General configuration"]')!);
    fireEvent.click(container.querySelector('button[aria-label="Edit"]')!);
    fireEvent.click(container.querySelector('button[aria-label="Delete"]')!);
    fireEvent.click(document.querySelector<HTMLButtonElement>(
      '.ant-popconfirm-buttons .ant-btn-primary',
    )!);
    expect(onAdd).toHaveBeenCalledOnce();
    expect(onConfig).toHaveBeenCalledOnce();
    expect(onEdit).toHaveBeenCalledWith(expect.objectContaining({
      identifiers: 'renewal.example.com',
    }));
    expect(onDelete).toHaveBeenCalledOnce();
  });

  it('renders the ACME form in the add-certificate modal', () => {
    renderCertificateForm();

    expect(screen.getByLabelText('Certificate remark')).toBeTruthy();
    expect(screen.getByLabelText('Token')).toBeTruthy();
    expect(screen.getByLabelText('Domain/IP list')).toBeTruthy();
    expect((screen.getByLabelText('Email') as HTMLInputElement).value).toBe(
      'default@example.com',
    );
    expect(screen.getByRole('button', { name: 'Issue certificate' })).toBeTruthy();
  });

  it.each([
    ['certificate-add-method', ['ACME']],
    ['certificate-authority', ['ZeroSSL', "Let's Encrypt"]],
    ['certificate-validation-method', ['Cloudflare']],
    ['certificate-key-type', ['EC256', 'EC384', 'RSA2048', 'RSA4096']],
    ['certificate-type', ['Domain certificate', 'IP certificate']],
  ])('provides the expected options for %s', (fieldId, expectedOptions) => {
    renderCertificateForm();
    expect(listSelectOptions(fieldId)).toEqual(expectedOptions);
  });

  it('renders and validates the general certificate configuration', () => {
    renderWithProviders(
      <CertificateConfigModal
        open
        config={{
          renewBeforeDays: 30,
          shortRenewBeforeHours: 24,
          shortCheckTimesPerDay: 4,
          checkTime: '05:00:00',
          defaultEmail: 'admin@example.com',
          globalPrivateKey: 'private-key',
        }}
        saving={false}
        onClose={() => {}}
        onSave={async () => true}
      />,
    );

    expect((screen.getByLabelText(
      'Renew certificate before expiry (days)',
    ) as HTMLInputElement).value).toBe('30');
    expect((screen.getByLabelText(
      'Renew short-lived certificate before expiry (hours)',
    ) as HTMLInputElement).value).toBe('24');
    expect((screen.getByLabelText('Daily ACME check time') as HTMLInputElement).value).toBe(
      '05:00:00',
    );
    expect((screen.getByLabelText(
      'ACME client default email',
    ) as HTMLInputElement).value).toBe('admin@example.com');
    expect(screen.getByLabelText('ACME global private key')).toBeTruthy();
    expect(listSelectOptions('certificate-short-check-times')).toEqual([
      '4 times per day',
      '5 times per day',
      '6 times per day',
    ]);
  });

  it('keeps IP certificates unavailable while Cloudflare is the only validator', () => {
    const parsed = CertificateIssueRequestSchema.safeParse({
      remark: 'IP',
      addMethod: 'acme',
      ca: 'letsencrypt',
      validationMethod: 'cloudflare',
      cloudflareToken: 'token',
      identifiers: '203.0.113.10',
      email: 'admin@example.com',
      keyType: 'EC256',
      certificateType: 'ip',
    });

    expect(parsed.success).toBe(false);
    if (!parsed.success) {
      expect(parsed.error.issues[0]?.message).toBe(
        'pages.certificates.validation.ipCloudflareUnsupported',
      );
    }
  });
});
