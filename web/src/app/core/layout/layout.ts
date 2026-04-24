import { Component, inject, computed } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterOutlet, RouterLink, RouterLinkActive } from '@angular/router';
import { AuthService } from '../services/auth.service';

import { UiService } from '../state/ui.service';

@Component({
  selector: 'app-layout',
  standalone: true,
  imports: [CommonModule, RouterOutlet, RouterLink, RouterLinkActive],
  templateUrl: './layout.html',
  styleUrls: ['./layout.css']
})
export class LayoutComponent {
  authService = inject(AuthService);
  uiService = inject(UiService);
  
  user = this.authService.user;
  sidebarCollapsed = this.uiService.sidebarCollapsed;
  toasts = this.uiService.toasts;
  
  navItems = [
    { label: 'Overview', icon: 'dashboard', path: '/' },
    { label: 'Connectors', icon: 'hub', path: '/connectors' },
    { label: 'Users', icon: 'people', path: '/users' },
    { label: 'Policies', icon: 'policy', path: '/policies' },
    { label: 'Audit Log', icon: 'list_alt', path: '/audit' },
    { label: 'Threats', icon: 'security', path: '/threats' },
    { label: 'Compliance', icon: 'assignment_turned_in', path: '/compliance' },
    { label: 'DLP', icon: 'search', path: '/dlp' },
    { label: 'Admin', icon: 'settings', path: '/admin' }
  ];

  onLogout(): void {
    this.authService.logout();
  }

  toggleSidebar(): void {
    this.uiService.toggleSidebar();
  }

  dismissToast(id: number): void {
    this.uiService.dismissToast(id);
  }
}
