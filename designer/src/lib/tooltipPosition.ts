// Pure placement math for the portaled Tooltip: flip to the side with room and
// clamp into the viewport so a tip near an edge (e.g. the top menu bar) is never
// clipped. Coords are final fixed-position top/left (no CSS transform), so the
// clamp is exact. Unit-agnostic px.

export interface AnchorRect {
  top: number;
  left: number;
  width: number;
  height: number;
}
export interface Size {
  width: number;
  height: number;
}
export interface Viewport {
  width: number;
  height: number;
}
export type Placement = "top" | "bottom";
export interface PositionedTooltip {
  top: number;
  left: number;
  placement: Placement;
}

function clamp(value: number, min: number, max: number): number {
  return Math.max(min, Math.min(max, value));
}

/** Resolve the tip's fixed top/left and the side it ends up on. Prefers
 *  `preferred`, flips when it doesn't fit, and falls back to the roomier side
 *  when neither fits. Horizontal: centred on the anchor, clamped to the
 *  viewport. `gap` is the anchor-to-tip offset, `margin` the viewport padding. */
export function resolveTooltipPosition(
  anchor: AnchorRect,
  tip: Size,
  viewport: Viewport,
  opts: { preferred?: Placement; gap?: number; margin?: number } = {},
): PositionedTooltip {
  const preferred = opts.preferred ?? "top";
  const gap = opts.gap ?? 6;
  const margin = opts.margin ?? 4;

  const fitsAbove = anchor.top - gap - tip.height >= margin;
  const fitsBelow = anchor.top + anchor.height + gap + tip.height <= viewport.height - margin;
  const spaceAbove = anchor.top;
  const spaceBelow = viewport.height - (anchor.top + anchor.height);

  let placement: Placement;
  if (preferred === "top") {
    placement = fitsAbove ? "top" : fitsBelow ? "bottom" : spaceAbove >= spaceBelow ? "top" : "bottom";
  } else {
    placement = fitsBelow ? "bottom" : fitsAbove ? "top" : spaceBelow >= spaceAbove ? "bottom" : "top";
  }

  const rawTop =
    placement === "top"
      ? anchor.top - gap - tip.height
      : anchor.top + anchor.height + gap;
  const top = clamp(rawTop, margin, Math.max(margin, viewport.height - margin - tip.height));

  const centerX = anchor.left + anchor.width / 2;
  const left = clamp(
    centerX - tip.width / 2,
    margin,
    Math.max(margin, viewport.width - margin - tip.width),
  );

  return { top, left, placement };
}
