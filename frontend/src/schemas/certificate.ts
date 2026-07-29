import { z } from 'zod';

const CertificateRequestBaseSchema = z.object({
  remark: z.string().trim().min(1, 'pages.certificates.validation.remarkRequired').max(120),
  addMethod: z.literal('acme'),
  ca: z.enum(['zerossl', 'letsencrypt']),
  validationMethod: z.literal('cloudflare'),
  cloudflareToken: z.string().trim().max(4096),
  identifiers: z.string().trim().min(1, 'pages.certificates.validation.identifiersRequired').max(16384),
  email: z.string().trim().email('pages.certificates.validation.emailInvalid').max(254),
  keyType: z.enum(['EC256', 'EC384', 'RSA2048', 'RSA4096']),
  certificateType: z.enum(['domain', 'ip']),
});

function rejectUnsupportedIP(
  value: z.infer<typeof CertificateRequestBaseSchema>,
  context: z.RefinementCtx,
) {
  if (value.certificateType === 'ip') {
    context.addIssue({
      code: 'custom',
      path: ['certificateType'],
      message: 'pages.certificates.validation.ipCloudflareUnsupported',
    });
  }
}

export const CertificateIssueRequestSchema = CertificateRequestBaseSchema.extend({
  cloudflareToken: z.string().trim().min(1, 'pages.certificates.validation.tokenRequired').max(4096),
}).superRefine(rejectUnsupportedIP);

export const CertificateUpdateRequestSchema = CertificateRequestBaseSchema
  .superRefine(rejectUnsupportedIP);

export const CertificateRecordSchema = z.object({
  id: z.number().int().positive(),
  remark: z.string(),
  addMethod: z.literal('acme'),
  ca: z.enum(['zerossl', 'letsencrypt']),
  validationMethod: z.literal('cloudflare'),
  identifiers: z.string(),
  certificateIdentifiers: z.string(),
  email: z.string(),
  keyType: z.enum(['EC256', 'EC384', 'RSA2048', 'RSA4096']),
  certificateType: z.enum(['domain', 'ip']),
  certificateFile: z.string(),
  keyFile: z.string(),
  issuedAt: z.string(),
  expiresAt: z.string(),
});

export const CertificateListSchema = z.array(CertificateRecordSchema);

export const CertificateConfigSchema = z.object({
  renewBeforeDays: z.number().int().min(1).max(90),
  shortRenewBeforeHours: z.number().int().min(24).max(168),
  shortCheckTimesPerDay: z.number().int().min(4).max(6),
  checkTime: z.string().regex(/^\d{2}:\d{2}:\d{2}$/, 'pages.certificates.validation.checkTimeInvalid'),
  defaultEmail: z.union([
    z.literal(''),
    z.string().trim().email('pages.certificates.validation.emailInvalid').max(254),
  ]),
  globalPrivateKey: z.string().trim().min(1, 'pages.certificates.validation.privateKeyRequired').max(16384),
});

export type CertificateIssueRequest = z.infer<typeof CertificateIssueRequestSchema>;
export type CertificateUpdateRequest = z.infer<typeof CertificateUpdateRequestSchema>;
export type CertificateRecord = z.infer<typeof CertificateRecordSchema>;
export type CertificateConfig = z.infer<typeof CertificateConfigSchema>;
