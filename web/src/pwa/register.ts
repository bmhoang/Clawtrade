export async function registerServiceWorker(): Promise<ServiceWorkerRegistration | null> {
  if (!('serviceWorker' in navigator)) return null;
  try {
    const registration = await navigator.serviceWorker.register('/sw.js');
    return registration;
  } catch {
    return null;
  }
}

export async function requestNotificationPermission(): Promise<boolean> {
  if (!('Notification' in window)) return false;
  const permission = await Notification.requestPermission();
  return permission === 'granted';
}

export async function subscribeToPush(registration: ServiceWorkerRegistration): Promise<PushSubscription | null> {
  try {
    const subscription = await registration.pushManager.subscribe({
      userVisibleOnly: true,
      applicationServerKey: new Uint8Array(0), // placeholder - needs VAPID key
    });
    return subscription;
  } catch {
    return null;
  }
}

export function isStandalone(): boolean {
  return window.matchMedia('(display-mode: standalone)').matches;
}

export function isPWAInstallable(): boolean {
  return 'serviceWorker' in navigator && 'PushManager' in window;
}
