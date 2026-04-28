import { TestBed } from '@angular/core/testing';
import { provideHttpClient } from '@angular/common/http';
import { provideHttpClientTesting, HttpTestingController } from '@angular/common/http/testing';
import { ThreatService } from './threat.service';
import { ApiService } from './api.service';

describe('ThreatService', () => {
  let service: ThreatService;
  let httpMock: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [ThreatService, ApiService, provideHttpClient(), provideHttpClientTesting()],
    });
    service = TestBed.inject(ThreatService);
    httpMock = TestBed.inject(HttpTestingController);
  });

  afterEach(() => {
    httpMock.verify();
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });

  it('should fetch alerts with correct params', () => {
    const mockAlerts = [{ id: '1', title: 'Test Alert', severity: 'HIGH' }];

    service.listAlerts({ status: 'OPEN' }).subscribe((alerts) => {
      expect(alerts).toEqual(mockAlerts as any);
    });

    const req = httpMock.expectOne(
      (req) => req.url.includes('/v1/threats/alerts') && req.params.has('status'),
    );
    expect(req.request.method).toBe('GET');
    expect(req.request.params.get('status')).toBe('OPEN');
    req.flush(mockAlerts);
  });
});
