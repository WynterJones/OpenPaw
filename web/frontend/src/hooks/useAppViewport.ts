/**
 * useAppViewport
 *
 * The app is a fixed shell, not a scrolling document — every screen sizes
 * itself to `--op-app-height`. On a phone the visible height is not
 * `100vh`: the URL bar collapses, and the on-screen keyboard eats the bottom
 * of the screen. Both show up on `window.visualViewport`, so that is what
 * drives the height.
 *
 * Zoom is deliberately ignored. Pinching (or iOS auto-zooming into a field
 * whose text is under 16px) also shrinks the visual viewport, and reacting to
 * that would re-lay-out the whole app at half size while the user is zoomed in.
 * Below 1.01 scale we trust the measurement; above it we keep the last one.
 */

import { useEffect } from 'react';

export function useAppViewport() {
  useEffect(() => {
    const root = document.documentElement;
    const vv = window.visualViewport;

    // Only the app shell locks the document; Login, Setup and the docs site
    // keep normal page scrolling.
    root.classList.add('op-shell-locked');

    const apply = () => {
      if (!vv) {
        root.style.setProperty('--op-app-height', `${window.innerHeight}px`);
        return;
      }
      if (vv.scale > 1.01) return;
      root.style.setProperty('--op-app-height', `${Math.round(vv.height)}px`);
    };

    apply();

    // Blurring a field on iOS can leave the page scrolled and zoomed with the
    // header out of view. Nothing in the app ever wants a scrolled document,
    // so snap it back whenever focus leaves a control.
    const resetScroll = () => {
      if (window.scrollY !== 0 || window.scrollX !== 0) window.scrollTo(0, 0);
    };

    window.addEventListener('resize', apply);
    window.addEventListener('orientationchange', apply);
    document.addEventListener('focusout', resetScroll);
    vv?.addEventListener('resize', apply);

    return () => {
      root.classList.remove('op-shell-locked');
      window.removeEventListener('resize', apply);
      window.removeEventListener('orientationchange', apply);
      document.removeEventListener('focusout', resetScroll);
      vv?.removeEventListener('resize', apply);
    };
  }, []);
}
