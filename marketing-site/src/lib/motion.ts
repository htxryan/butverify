/**
 * Reduced-motion helper for JS-driven motion.
 *
 * The CSS gate in global.css already strips animation/transition from the
 * whole document when the user has set system reduce-motion. This helper
 * is the JS twin: any code that drives motion imperatively (scroll, focus
 * step, IntersectionObserver-triggered class flips, programmatic
 * Element.animate calls, etc.) should branch on it and skip the animation
 * path entirely, going straight to the final state.
 *
 * Returns false during SSR (no window) so server-rendered output never
 * pre-suppresses motion the client might otherwise want.
 */
export function prefersReducedMotion(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') {
    return false;
  }
  return window.matchMedia('(prefers-reduced-motion: reduce)').matches;
}
