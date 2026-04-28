import { TestBed } from '@angular/core/testing';
import { SseService } from './sse.service';
import { AuthService } from './auth.service';
import { NgZone } from '@angular/core';

describe('SseService', () => {
  let service: SseService;
  let mockAuthService: any;
  let mockEventSource: any;

  beforeEach(() => {
    mockAuthService = {
      getCurrentOrgId: jasmine.createSpy('getCurrentOrgId').and.returnValue('org-123'),
    };

    // Mock global EventSource
    mockEventSource = {
      close: jasmine.createSpy('close'),
      onmessage: null,
      onerror: null,
    };
    (window as any).EventSource = jasmine.createSpy('EventSource').and.returnValue(mockEventSource);

    TestBed.configureTestingModule({
      providers: [SseService, { provide: AuthService, useValue: mockAuthService }],
    });
    service = TestBed.inject(SseService);
  });

  it('should be created', () => {
    expect(service).toBeTruthy();
  });

  it('should create EventSource with correct URL and credentials', () => {
    service.connect();

    expect(window.EventSource).toHaveBeenCalledWith(
      jasmine.stringMatching(/\/audit\/v1\/events\/stream\?org_id=org-123/),
      jasmine.objectContaining({ withCredentials: true }),
    );
  });

  it('should handle incoming messages and parse JSON', (done) => {
    service.connect().subscribe((data) => {
      expect(data).toEqual({ event: 'test' });
      done();
    });

    // Simulate message
    if (mockEventSource.onmessage) {
      mockEventSource.onmessage({ data: JSON.stringify({ event: 'test' }) });
    }
  });

  it('should close existing connection when reconnecting', () => {
    service.connect();
    service.connect();

    expect(mockEventSource.close).toHaveBeenCalledTimes(1);
  });

  it('should close connection on disconnect', () => {
    service.connect();
    service.disconnect();

    expect(mockEventSource.close).toHaveBeenCalled();
  });
});
