const SOUND_ENABLED_KEY = 'openpaw_notification_sound';
const SOUND_VOLUME_KEY = 'openpaw_notification_volume';

export function isNotificationSoundEnabled(): boolean {
  return localStorage.getItem(SOUND_ENABLED_KEY) !== 'false';
}

export function setNotificationSoundEnabled(enabled: boolean) {
  localStorage.setItem(SOUND_ENABLED_KEY, enabled ? 'true' : 'false');
}

export function getNotificationVolume(): number {
  const stored = localStorage.getItem(SOUND_VOLUME_KEY);
  return stored ? parseFloat(stored) : 0.5;
}

export function setNotificationVolume(volume: number) {
  const clamped = Math.max(0, Math.min(1, volume));
  localStorage.setItem(SOUND_VOLUME_KEY, clamped.toString());
  if (cachedAudio) {
    cachedAudio.volume = clamped;
  }
}

let cachedAudio: HTMLAudioElement | null = null;

export function playNotificationSound() {
  if (!isNotificationSoundEnabled()) return;
  if (!cachedAudio) {
    cachedAudio = new Audio('/sounds/notification.mp3');
  }
  cachedAudio.volume = getNotificationVolume();
  cachedAudio.currentTime = 0;
  cachedAudio.play().catch(() => {});
}

export async function requestNotificationPermission(): Promise<boolean> {
  if (!('Notification' in window)) return false;
  if (Notification.permission === 'granted') return true;
  if (Notification.permission === 'denied') return false;

  const result = await Notification.requestPermission();
  return result === 'granted';
}

export function showBrowserNotification(title: string, options?: { body?: string; icon?: string; tag?: string }) {
  if (!('Notification' in window) || Notification.permission !== 'granted') return;
  if (document.hasFocus()) return;

  new Notification(title, {
    body: options?.body,
    icon: options?.icon || '/cat-toolbar.webp',
    tag: options?.tag || 'openpaw-notification',
  });
}
