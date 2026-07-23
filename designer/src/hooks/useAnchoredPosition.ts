import { useEffect, useLayoutEffect, useRef, useState } from "react";
import type { RefObject } from "react";

/**
 * Fixed-coord positioning for portaled overlays anchored to a trigger. Tracks
 * `anchorRef` while `open` across scroll (capture, so ancestor panels count)
 * and resize, so the scrollable panel can't clip the overlay. Re-measures only
 * on open/scroll/resize, not on `measure` identity change, so keep
 * placement-affecting inputs stable per open.
 */
export function useAnchoredPosition<T>(
  anchorRef: RefObject<HTMLElement | null>,
  open: boolean,
  measure: (rect: DOMRect) => T,
): T | null {
  const [pos, setPos] = useState<T | null>(null);
  // Latest measure via ref so callers don't memoize (React Compiler owns that).
  const measureRef = useRef(measure);
  useEffect(() => {
    measureRef.current = measure;
  });

  useLayoutEffect(() => {
    if (!open) return;
    let frame = 0;
    const place = () => {
      const rect = anchorRef.current?.getBoundingClientRect();
      if (rect) setPos(measureRef.current(rect));
    };
    const schedule = () => {
      cancelAnimationFrame(frame);
      frame = requestAnimationFrame(place);
    };
    place();
    window.addEventListener("scroll", schedule, true);
    window.addEventListener("resize", schedule);
    return () => {
      cancelAnimationFrame(frame);
      window.removeEventListener("scroll", schedule, true);
      window.removeEventListener("resize", schedule);
    };
  }, [open, anchorRef]);

  return open ? pos : null;
}
