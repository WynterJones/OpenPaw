export interface PathInsertionTarget {
  id: string;
  label: string;
  insert: (path: string) => void;
}

let activeTarget: PathInsertionTarget | null = null;

export function activatePathInsertionTarget(target: PathInsertionTarget) {
  activeTarget = target;
}

export function clearPathInsertionTarget(id: string) {
  if (activeTarget?.id === id) activeTarget = null;
}

export function getPathInsertionTarget() {
  return activeTarget;
}
