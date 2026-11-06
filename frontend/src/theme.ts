import { GetSystemAccentColor } from "../wailsjs/go/main/App";
import { WindowSetBackgroundColour } from "../wailsjs/runtime/runtime";

/** Accent handling: the popup paints the selected row with the accent as a
 *  solid fill, so the text colour on top has to be chosen from the accent's
 *  luminance instead of assumed white. */

type Rgb = [number, number, number];

function parseHex(hex: string): Rgb | null {
  const match = /^#?([0-9a-f]{6})$/i.exec(hex.trim());
  if (!match) return null;
  const value = parseInt(match[1], 16);
  return [(value >> 16) & 255, (value >> 8) & 255, value & 255];
}

function toLinear(channel: number) {
  const c = channel / 255;
  return c <= 0.04045 ? c / 12.92 : Math.pow((c + 0.055) / 1.055, 2.4);
}

function luminance(hex: string): number | null {
  const rgb = parseHex(hex);
  if (!rgb) return null;
  return 0.2126 * toLinear(rgb[0]) + 0.7152 * toLinear(rgb[1]) + 0.0722 * toLinear(rgb[2]);
}

function contrastRatio(a: string, b: string): number {
  const la = luminance(a);
  const lb = luminance(b);
  if (la === null || lb === null) return 0;
  const [hi, lo] = la >= lb ? [la, lb] : [lb, la];
  return (hi + 0.05) / (lo + 0.05);
}

/** Pick the text colour (from `candidates`) that contrasts best with `hex`. */
function contrastText(hex: string, candidates: string[] = ["#ffffff", "#0b0b0b"]): string {
  let best = candidates[0];
  let bestRatio = 0;
  for (const candidate of candidates) {
    const ratio = contrastRatio(hex, candidate);
    if (ratio > bestRatio) {
      best = candidate;
      bestRatio = ratio;
    }
  }
  return best;
}

/** Apply an accent to the whole UI; `undefined` restores the theme default. */
function applyAccent(hex?: string) {
  const root = document.documentElement;
  const accent = (hex || "").trim();
  if (!accent || !parseHex(accent)) {
    root.style.removeProperty("--accent");
    root.style.removeProperty("--accent-contrast");
    return;
  }
  root.style.setProperty("--accent", accent);
  root.style.setProperty("--accent-contrast", contrastText(accent));
}

/** Read the Windows accent colour and apply it, or fall back to the theme default. */
export async function refreshAccent(): Promise<void> {
  try {
    const accent = await GetSystemAccentColor();
    applyAccent(accent || undefined);
  } catch {
    applyAccent(undefined);
  }
}

/** Keep the native window backdrop in step with the theme: it shows for a beat
 *  while the WebView paints (and behind rounded corners). */
export function syncWindowBackground() {
  const value = getComputedStyle(document.documentElement).getPropertyValue("--bg-primary").trim();
  const match = /rgba?\((\d+),\s*(\d+),\s*(\d+)/.exec(value);
  if (!match) return;
  try {
    WindowSetBackgroundColour(Number(match[1]), Number(match[2]), Number(match[3]), 255);
  } catch {
    /* not running under Wails (browser prototype) */
  }
}
