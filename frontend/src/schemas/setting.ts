import { z } from 'zod';

const port = z.number().int().min(1).max(65535);

export const AllSettingSchema = z.object({
  webListen: z.string().optional(),
  webDomain: z.string().optional(),
  webPort: port.optional(),
  webCertFile: z.string().optional(),
  webKeyFile: z.string().optional(),
  webBasePath: z.string().regex(/^\//, 'pages.settings.validation.pathLeadingSlash').optional(),
  sessionMaxAge: z.number().int().min(1).max(525600).optional(),
  trustedProxyCIDRs: z.string().optional(),
  panelOutbound: z.string().optional(),

  pageSize: z.number().int().min(0).max(1000).optional(),
  expireDiff: z.number().int().min(0).optional(),
  trafficDiff: z.number().int().min(0).max(100).optional(),
  datepicker: z.enum(['gregorian', 'jalalian']).optional(),
  timeLocation: z.string().optional(),
  twoFactorEnable: z.boolean().optional(),
  twoFactorToken: z.string().optional(),
  restartXrayOnClientDisable: z.boolean().optional(),
  warpUpdateInterval: z.number().int().min(0).optional(),

  hasTwoFactorToken: z.boolean().optional(),
  hasWarpSecret: z.boolean().optional(),
  hasNordSecret: z.boolean().optional(),
}).loose();

export type AllSettingInput = z.infer<typeof AllSettingSchema>;
