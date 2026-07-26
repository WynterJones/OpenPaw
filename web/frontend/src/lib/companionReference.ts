/**
 * The reference-image selection used when generating a companion.
 *
 * Kept out of the component file because exporting a value alongside a
 * component breaks Fast Refresh — the module reloads as a component module and
 * the constant's identity churns with it.
 */

export interface ReferenceState {
  /** Data URI of the reference, or null for none. */
  image: string | null;
  /** PixelLab init_image_strength, 1..999. */
  strength: number;
  /** Where it came from — shown so the choice stays legible. */
  label: string;
}

/** PixelLab's own default strength; 300 treats the reference as strong guidance. */
export const NO_REFERENCE: ReferenceState = { image: null, strength: 300, label: '' };
