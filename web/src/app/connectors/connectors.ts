import { Component } from '@angular/core';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-connectors',
  standalone: true,
  imports: [CommonModule],
  templateUrl: './connectors.html',
  styleUrl: './connectors.css'
})
export class ConnectorsComponent {
  connectors = [
    {
      name: 'Acme HR Sync',
      status: 'Active',
      scopes: ['scim:write', 'events:write'],
      createdDate: 'Apr 6, 2026',
      eventVolume: '45,210'
    }
  ];
}
