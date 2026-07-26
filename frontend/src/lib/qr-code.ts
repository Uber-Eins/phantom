const VERSION_40_LOW_CAPACITY = {
  numeric: 7089,
  alphanumeric: 4296,
  byte: 2953,
} as const;

const NUMERIC_RE = /^[0-9]+$/;
const ALPHANUMERIC_RE = /^[0-9A-Z $%*+\-./:]+$/;
const UTF8_ENCODER = new TextEncoder();

/**
 * Whether a value fits in one version-40 QR code at error-correction level L.
 * This mirrors the segment selection used by Ant Design's QR encoder.
 */
export function canEncodeQrCode(value: string): boolean {
  if (value.length === 0) return false;
  if (NUMERIC_RE.test(value)) {
    return value.length <= VERSION_40_LOW_CAPACITY.numeric;
  }
  if (ALPHANUMERIC_RE.test(value)) {
    return value.length <= VERSION_40_LOW_CAPACITY.alphanumeric;
  }
  return UTF8_ENCODER.encode(value).length <= VERSION_40_LOW_CAPACITY.byte;
}
