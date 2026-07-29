import { useMutation, useQueryClient } from '@tanstack/react-query';

import { CertificateRecordSchema } from '@/schemas/certificate';
import type {
  CertificateIssueRequest,
  CertificateUpdateRequest,
} from '@/schemas/certificate';
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

export function useUpdateCertificate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      request,
    }: {
      id: number;
      request: CertificateUpdateRequest;
    }) => {
      const message = await HttpUtil.post(`/panel/api/certificates/update/${id}`, request);
      return parseMsg(message, CertificateRecordSchema, 'certificates/update');
    },
    onSuccess: (message) => {
      if (message.success) {
        queryClient.invalidateQueries({ queryKey: keys.certificates.list() });
      }
    },
  });
}

export function useDeleteCertificate() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: number) => HttpUtil.post(`/panel/api/certificates/delete/${id}`),
    onSuccess: (message) => {
      if (message.success) {
        queryClient.invalidateQueries({ queryKey: keys.certificates.list() });
      }
    },
  });
}
