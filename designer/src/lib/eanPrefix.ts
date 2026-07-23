/**
 * GS1 prefix hint for EAN-13: the first 3 digits map to the GS1 member
 * organisation (country) or a special-use range. Curated common ranges;
 * uncovered prefixes return null (no hint). Pure, no UI.
 *
 * The prefix identifies the GS1 member organisation that issued the number,
 * NOT the country of manufacture and NOT any validity guarantee, so this is a
 * hint only, never a validation input.
 */

export type EanPrefixKey =
  | "usCa"
  | "restricted"
  | "coupon"
  | "fr"
  | "de"
  | "jp"
  | "ru"
  | "gb"
  | "cn"
  | "ch"
  | "it"
  | "es"
  | "nl"
  | "at"
  | "au"
  | "issn"
  | "isbn"
  | "refund";

interface PrefixRange {
  lo: number;
  hi: number;
  key: EanPrefixKey;
}

// By the numeric value of the 3-digit prefix (000-999). First match wins.
// Curated from the GS1 prefix list; uncovered ranges (e.g. 140-199) return null.
const RANGES: readonly PrefixRange[] = [
  { lo: 0, hi: 19, key: "usCa" },
  { lo: 20, hi: 29, key: "restricted" },
  { lo: 30, hi: 39, key: "usCa" },
  { lo: 40, hi: 49, key: "restricted" },
  { lo: 50, hi: 59, key: "coupon" },
  { lo: 60, hi: 139, key: "usCa" },
  { lo: 200, hi: 299, key: "restricted" },
  { lo: 300, hi: 379, key: "fr" },
  { lo: 400, hi: 440, key: "de" },
  { lo: 450, hi: 459, key: "jp" },
  { lo: 460, hi: 469, key: "ru" },
  { lo: 490, hi: 499, key: "jp" },
  { lo: 500, hi: 509, key: "gb" },
  { lo: 690, hi: 699, key: "cn" },
  { lo: 760, hi: 769, key: "ch" },
  { lo: 800, hi: 839, key: "it" },
  { lo: 840, hi: 849, key: "es" },
  { lo: 870, hi: 879, key: "nl" },
  { lo: 900, hi: 919, key: "at" },
  { lo: 930, hi: 939, key: "au" },
  { lo: 977, hi: 977, key: "issn" },
  { lo: 978, hi: 979, key: "isbn" },
  { lo: 980, hi: 980, key: "refund" },
  { lo: 981, hi: 984, key: "coupon" },
  { lo: 990, hi: 999, key: "coupon" },
];

/** Locale key for the GS1 prefix of `digits` (needs the first 3), or null. */
export function eanPrefixKey(digits: string): EanPrefixKey | null {
  if (!/^\d{3}/.test(digits)) return null;
  const n = parseInt(digits.slice(0, 3), 10);
  for (const r of RANGES) if (n >= r.lo && n <= r.hi) return r.key;
  return null;
}
