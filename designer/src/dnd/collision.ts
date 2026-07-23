import { pointerWithin, closestCenter, type CollisionDetection } from '@dnd-kit/core';
import { CANVAS_DROPPABLE_ID, ROW_PREFIX, isRowDragId } from './types';

/** One DndContext serves two jobs: reorder the curated palette rows and spawn
 *  objects by dragging onto the canvas. The curation `editing` flag keeps the
 *  two row-drag meanings mutually exclusive so they never compete:
 *  - editing: a row drag reorders only (closest-center among rows); the canvas
 *    is never a target, so dragging out spawns nothing.
 *  - not editing: a row drag spawns only; it hits the canvas when the pointer is
 *    inside it, and never reports a sibling row (no stray reorder preview).
 *  Default rectIntersection fails for the reorder case: the dragged row's own
 *  slot keeps the largest overlap, so `over` never leaves the active id.
 *  Any other drag (flat/search entries) hits the canvas only via pointerWithin. */
export const makePaletteCollision =
  (editing: boolean): CollisionDetection =>
  (args) => {
    if (isRowDragId(String(args.active.id))) {
      if (editing)
        return closestCenter({
          ...args,
          droppableContainers: args.droppableContainers.filter((c) =>
            String(c.id).startsWith(ROW_PREFIX),
          ),
        });
      return pointerWithin(args).filter((c) => c.id === CANVAS_DROPPABLE_ID);
    }
    return pointerWithin(args);
  };
