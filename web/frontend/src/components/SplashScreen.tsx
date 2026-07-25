import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from '../lib/api';
import { isTauri } from '../lib/tauri';

const BG_SRC = '/preset-bg/bg-8.webp';
const LOGO_SRC = '/logo-transparent.png';
const CAT_SRC = '/cat-toolbar.webp';

/* Keep in sync with the .op-splash-* animation delays in index.css. */
const HOLD_MS = 3000;
const EXIT_MS = 600;
const REDUCED_HOLD_MS = 1200;
const REDUCED_EXIT_MS = 250;

/* Decoding three local images is normally a few milliseconds; the cap only
   exists so a cold cache can never leave the user staring at a black frame. */
const DECODE_CAP_MS = 500;

/* Per tab session, so the splash plays on a real app launch (and every desktop
   launch) but not on the reloads of an editing session. */
const SEEN_KEY = 'openpaw:splash-seen';

function alreadyPlayed(): boolean {
  // The desktop app shows a real splash *window* before this document exists,
  // so replaying it here would splash twice. This overlay is for the browser
  // (npx) case, which has no window to show.
  if (isTauri()) return true;
  try {
    return sessionStorage.getItem(SEEN_KEY) === '1';
  } catch {
    return false;
  }
}

function markPlayed() {
  try {
    sessionStorage.setItem(SEEN_KEY, '1');
  } catch {
    /* private mode -- the splash simply replays */
  }
}

function preload(src: string): Promise<void> {
  const img = new Image();
  img.src = src;
  return img.decode().catch(() => {});
}

export function SplashScreen() {
  const [visible, setVisible] = useState(() => !alreadyPlayed());
  const [ready, setReady] = useState(false);
  const [leaving, setLeaving] = useState(false);
  const [version, setVersion] = useState('');
  const [reduced] = useState(
    () => typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches,
  );
  const exiting = useRef(false);

  const dismiss = useCallback(() => {
    if (exiting.current) return;
    exiting.current = true;
    setLeaving(true);
    setTimeout(() => setVisible(false), reduced ? REDUCED_EXIT_MS : EXIT_MS);
  }, [reduced]);

  useEffect(() => {
    if (!visible) return;
    markPlayed();

    let alive = true;
    const timers: number[] = [];

    Promise.race([
      Promise.all([preload(BG_SRC), preload(LOGO_SRC), preload(CAT_SRC)]),
      new Promise((r) => timers.push(window.setTimeout(r, DECODE_CAP_MS))),
    ]).then(() => {
      if (!alive) return;
      setReady(true);
      timers.push(window.setTimeout(dismiss, reduced ? REDUCED_HOLD_MS : HOLD_MS));
    });

    window.addEventListener('keydown', dismiss);
    return () => {
      alive = false;
      timers.forEach(clearTimeout);
      window.removeEventListener('keydown', dismiss);
    };
  }, [visible, reduced, dismiss]);

  useEffect(() => {
    if (!visible) return;
    let alive = true;
    api
      .getQuiet<{ version: string }>('/system/info')
      .then((d) => alive && setVersion(d.version || ''))
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, [visible]);

  if (!visible) return null;

  return (
    <div
      className="op-splash"
      data-phase={leaving ? 'exit' : 'enter'}
      data-motion={reduced ? 'reduced' : 'full'}
      onClick={dismiss}
      aria-hidden="true"
    >
      {ready && (
        <>
          <div className="op-splash-bg" style={{ backgroundImage: `url(${BG_SRC})` }} />
          <div className="op-splash-scrim" />
          <div className="op-splash-glow" />
          <img className="op-splash-logo" src={LOGO_SRC} alt="" />
          <img className="op-splash-cat" src={CAT_SRC} alt="" />
          <div className="op-splash-version">{version && `v${version}`}</div>
        </>
      )}
    </div>
  );
}
