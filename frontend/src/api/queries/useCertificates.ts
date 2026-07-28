import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import {
  CertificateConfigSchema,
  CertificateListSchema,
  type CertificateConfig,
} from '@/schemas/certificate';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';

async function fetchCertificates() {
  const message = await HttpUtil.get('/panel/api/certificates/list', undefined, { silent: true });
  if (!message.success) throw new Error(message.msg || 'Failed to fetch certificates');
  return parseMsg(message, CertificateListSchema, 'certificates/list').obj ?? [];
}

async function fetchCertificateConfig() {
  const message = await HttpUtil.get('/panel/api/certificates/config', undefined, { silent: true });
  if (!message.success) throw new Error(message.msg || 'Failed to fetch certificate configuration');
  const config = parseMsg(message, CertificateConfigSchema, 'certificates/config').obj;
  if (!config) throw new Error('Certificate configuration is missing');
  return config;
}

export function useCertificates() {
  return useQuery({
    queryKey: keys.certificates.list(),
    queryFn: fetchCertificates,
    staleTime: Infinity,
  });
}

export function useCertificateConfig() {
  const queryClient = useQueryClient();
  const query = useQuery({
    queryKey: keys.certificates.config(),
    queryFn: fetchCertificateConfig,
    staleTime: Infinity,
  });
  const save = useMutation({
    mutationFn: async (config: CertificateConfig) => {
      const message = await HttpUtil.post('/panel/api/certificates/config', config);
      return parseMsg(message, CertificateConfigSchema, 'certificates/config/save');
    },
    onSuccess: (message) => {
      if (message.success) {
        queryClient.setQueryData(keys.certificates.config(), message.obj);
      }
    },
  });
  return { ...query, save };
}
