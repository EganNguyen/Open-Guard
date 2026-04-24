import { Injectable, signal, effect, inject, PLATFORM_ID } from '@angular/core';
import { isPlatformBrowser } from '@angular/common';
import { Toast } from '../models/ui.model';

@Injectable({ providedIn: 'root' })
export class UiService {
  private platformId = inject(PLATFORM_ID);
  
  readonly sidebarCollapsed = signal(false);

  readonly activeDrawer = signal<{
    type: 'audit' | 'alert' | null;
    id: string | null;
  }>({ type: null, id: null });

  readonly toasts = signal<Toast[]>([]);

  constructor() {
    if (isPlatformBrowser(this.platformId)) {
      const saved = localStorage.getItem('og:ui:sidebar');
      if (saved) {
        this.sidebarCollapsed.set(JSON.parse(saved));
      }
      
      effect(() => {
        localStorage.setItem('og:ui:sidebar', JSON.stringify(this.sidebarCollapsed()));
      });
    }
  }

  toggleSidebar() {
    this.sidebarCollapsed.update(v => !v);
  }

  openDrawer(type: 'audit' | 'alert', id: string) {
    this.activeDrawer.set({ type, id });
  }

  closeDrawer() {
    this.activeDrawer.set({ type: null, id: null });
  }

  addToast(toast: Toast) {
    const id = Date.now();
    this.toasts.update(ts => [...ts, { ...toast, id }]);
    setTimeout(() => this.dismissToast(id), toast.duration ?? 4000);
  }

  dismissToast(id: number) {
    this.toasts.update(ts => ts.filter(t => t.id !== id));
  }
}
