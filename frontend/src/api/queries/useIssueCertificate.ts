import { useMutation, useQueryClient } from '@tanstack/react-query';

import { CertificateRecordSchema } from '@/schemas/certificate';
import type { CertificateIssueRequest } from '@/schemas/certificate';
import { HttpUtil } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import { keys } from '@/api/queryKeys';

export function useIssueCertificate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (request: CertificateIssueRequest) => {
      const message = await HttpUtil.post('/panel/api/certificates/issue', request);
      return parseMsg(message, CertificateRecordSchema, 'certificates/issue');
    },
    onSuccess: (message) => {
      if (message.success) {
        queryClient.invalidateQueries({ queryKey: keys.certificates.list() });
      }
    },
  });
}
