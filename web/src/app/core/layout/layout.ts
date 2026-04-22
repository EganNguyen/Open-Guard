import { Component, inject, computed } from '@angular/core';
import { CommonModule } from '@angular/common';
import { RouterOutlet, RouterLink, RouterLinkActive } from '@angular/router';
import { AuthService } from '../services/auth.service';

@Component({
  selector: 'app-layout',
  standalone: true,
  imports: [CommonModule, RouterOutlet, RouterLink, RouterLinkActive],
  templateUrl: './layout.html',
  styleUrls: ['./layout.css']
})
export class LayoutComponent {
  authService = inject(AuthService);
  user = this.authService.user;
  
  navItems = [
    { label: 'Overview', icon: 'dashboard', path: '/' },
    { label: 'Connectors', icon: 'hub', path: '/connectors' },
    { label: 'Users', icon: 'people', path: '/users' },
    { label: 'Policies', icon: 'policy', path: '/policies' },
    { label: 'Audit Log', icon: 'list_alt', path: '/audit' },
    { label: 'Threats', icon: 'security', path: '/threats' },
    { label: 'Compliance', icon: 'assignment_turned_in', path: '/compliance' }
  ];

  onLogout(): void {
    this.authService.logout();
  }
}
