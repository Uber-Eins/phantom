import { ObjectUtil } from '@/utils';

export class AllSetting {
  webListen = '';
  webDomain = '';
  webPort = 2053;
  webCertFile = '';
  webKeyFile = '';
  webBasePath = '/';
  sessionMaxAge = 360;
  trustedProxyCIDRs = '127.0.0.1/32,::1/128';
  panelOutbound = '';

  pageSize = 25;
  expireDiff = 0;
  trafficDiff = 0;
  datepicker: 'gregorian' | 'jalalian' = 'gregorian';
  timeLocation = 'Local';
  twoFactorEnable = false;
  twoFactorToken = '';
  restartXrayOnClientDisable = true;
  warpUpdateInterval = 0;

  hasTwoFactorToken = false;
  hasWarpSecret = false;
  hasNordSecret = false;

  constructor(data?: unknown) {
    if (data != null) {
      ObjectUtil.cloneProps(this, data);
    }
  }

  equals(other: AllSetting): boolean {
    return ObjectUtil.equals(this, other);
  }
}
