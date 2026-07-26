import { sha256 } from '@noble/hashes/sha2.js';
import { bytesToHex, utf8ToBytes } from '@noble/hashes/utils.js';

// Mirrors deriveSpiderX in internal/share byte-for-byte so frontend and
// backend connection links agree; returns '' when there is no seed and no
// client key.
export function deriveSpiderX(seed: string, clientKey: string): string {
  if (!seed && !clientKey) return '';
  return `/${bytesToHex(sha256(utf8ToBytes(`${seed}|${clientKey}`))).slice(0, 15)}`;
}
