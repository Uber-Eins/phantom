export interface RealityScanResult {
  alpn: string;
  certIssuer: string;
  certSubject: string;
  certValid: boolean;
  curveID: string;
  feasible: boolean;
  h2: boolean;
  host: string;
  ip: string;
  latencyMs: number;
  notAfter: string;
  port: number;
  reason: string;
  serverNames: string[];
  target: string;
  tls13: boolean;
  tlsVersion: string;
  x25519: boolean;
}
