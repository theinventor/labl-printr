// DOM <-> plain-text + caret roundtrip. <br> = \n; trailing <br> is
// Chrome's empty-line placeholder and is dropped (segmentsToHTML emits one).
//
// Markers render as atomic widgets: a `contenteditable="false"` element whose
// canonical `«…»` text lives in `data-m` while its visible content is a
// friendlier chip (name / clock icon + label). The walkers below read `data-m`
// and treat the widget as one indivisible unit so the plain-text roundtrip and
// caret offsets stay aligned with the canonical string, not the rendered chip.

function markerText(n: Node): string | null {
  if (n.nodeType !== Node.ELEMENT_NODE) return null;
  const el = n as Element;
  // getAttribute returns null when absent; typeof guard keeps the lightweight
  // test mocks (which omit it) working without jsdom.
  return typeof el.getAttribute === "function" ? el.getAttribute("data-m") : null;
}

export function domToPlainText(root: Node): string {
  let out = "";
  const walk = (n: Node) => {
    if (n.nodeType === Node.TEXT_NODE) {
      out += n.nodeValue ?? "";
      return;
    }
    if (n.nodeType !== Node.ELEMENT_NODE) return;
    const marker = markerText(n);
    if (marker !== null) {
      out += marker;
      return;
    }
    if ((n as Element).tagName === "BR") {
      out += "\n";
      return;
    }
    n.childNodes.forEach(walk);
  };
  const lastIdx = root.childNodes.length - 1;
  root.childNodes.forEach((c, i) => {
    if (i === lastIdx && c.nodeType === Node.ELEMENT_NODE && (c as Element).tagName === "BR") {
      return;
    }
    walk(c);
  });
  return out;
}

/** Mirrors domToPlainText walk order. */
export function getCaretOffset(root: Node, node: Node, offset: number): number {
  let count = 0;
  let found = false;
  const visit = (n: Node) => {
    if (found) return;
    if (n === node) {
      if (n.nodeType === Node.TEXT_NODE) {
        count += offset;
        found = true;
        return;
      }
      // Caret reported on a widget element (boundary): offset 0 = before, any
      // positive index = after. Caret can't sit inside (contenteditable=false).
      const marker = markerText(n);
      if (marker !== null) {
        count += offset > 0 ? marker.length : 0;
        found = true;
        return;
      }
      // Element offset counts child positions.
      for (let i = 0; i < offset && i < n.childNodes.length; i += 1) {
        const child = n.childNodes[i];
        if (child) visit(child);
      }
      found = true;
      return;
    }
    if (n.nodeType === Node.TEXT_NODE) {
      count += (n.nodeValue ?? "").length;
      return;
    }
    if (n.nodeType !== Node.ELEMENT_NODE) return;
    const marker = markerText(n);
    if (marker !== null) {
      count += marker.length;
      return;
    }
    if ((n as Element).tagName === "BR") {
      count += 1;
      return;
    }
    n.childNodes.forEach(visit);
  };
  visit(root);
  return count;
}

/** Inverse of getCaretOffset; clamps to end when overshooting. */
export function findCaretPosition(
  root: Node,
  target: number,
): { node: Node; offset: number } {
  // A negative target would otherwise yield a negative text-node offset and
  // throw when fed to a Range; clamp to the field start.
  let remaining = Math.max(0, target);
  let result: { node: Node; offset: number } | null = null;
  const atBoundary = (el: Element, after: boolean): { node: Node; offset: number } | null => {
    const parent = el.parentNode;
    if (!parent) return null;
    const idx = Array.prototype.indexOf.call(parent.childNodes, el);
    return { node: parent, offset: after ? idx + 1 : idx };
  };
  const visit = (n: Node) => {
    if (result) return;
    if (n.nodeType === Node.TEXT_NODE) {
      const len = (n.nodeValue ?? "").length;
      if (remaining <= len) {
        result = { node: n, offset: remaining };
        return;
      }
      remaining -= len;
      return;
    }
    if (n.nodeType !== Node.ELEMENT_NODE) return;
    const marker = markerText(n);
    if (marker !== null) {
      const len = marker.length;
      // Atomic: land before (remaining<=0) or after (anywhere up to/including
      // its end); a position "inside" snaps to after rather than splitting it.
      if (remaining <= 0) {
        result = atBoundary(n as Element, false);
        return;
      }
      if (remaining <= len) {
        result = atBoundary(n as Element, true);
        return;
      }
      remaining -= len;
      return;
    }
    const el = n as Element;
    if (el.tagName === "BR") {
      if (remaining === 0) {
        const parent = el.parentNode;
        if (!parent) return;
        const idx = Array.prototype.indexOf.call(parent.childNodes, el);
        result = { node: parent, offset: idx };
        return;
      }
      remaining -= 1;
      if (remaining === 0) {
        // Land inside next sibling: Chrome snaps (parent, brIndex+1) back into prev text.
        const next = el.nextSibling;
        if (next && next.nodeType === Node.TEXT_NODE) {
          result = { node: next, offset: 0 };
          return;
        }
        if (next && next.nodeType === Node.ELEMENT_NODE) {
          result = { node: next, offset: 0 };
          return;
        }
        const parent = el.parentNode;
        if (!parent) return;
        const idx = Array.prototype.indexOf.call(parent.childNodes, el);
        result = { node: parent, offset: idx + 1 };
        return;
      }
      return;
    }
    n.childNodes.forEach(visit);
  };
  visit(root);
  if (result) return result;
  return { node: root, offset: root.childNodes.length };
}
