import { Injectable, inject } from '@angular/core';
import { ApiService } from './api.service';
import { Observable, map } from 'rxjs';

import { Connector, ConnectorRegistrationResult } from '../models/connector.model';

export interface ConnectorUI extends Connector {
  status?: string;
  scopes?: string[];
  createdDate?: string;
  eventVolume?: string;
}

@Injectable({
  providedIn: 'root',
})
export class ConnectorService {
  private api = inject(ApiService);

  listConnectors(): Observable<ConnectorUI[]> {
    return this.api.get<Connector[]>('/mgmt/connectors').pipe(
      map((data) =>
        Array.isArray(data)
          ? data.map((c) => ({
              ...c,
              status: 'Active',
              scopes: ['openid', 'profile', 'email'],
              createdDate: 'Apr 17, 2026',
              eventVolume: '0',
            }))
          : [],
      ),
    );
  }

  createConnector(connector: Partial<Connector>): Observable<ConnectorRegistrationResult> {
    return this.api.post<ConnectorRegistrationResult>('/mgmt/connectors', connector);
  }

  updateConnector(id: string, connector: Partial<Connector>): Observable<{ status: string }> {
    return this.api.put<{ status: string }>(`/mgmt/connectors/${id}`, connector);
  }

  deleteConnector(id: string): Observable<{ status: string }> {
    return this.api.delete<{ status: string }>(`/mgmt/connectors/${id}`);
  }
}
