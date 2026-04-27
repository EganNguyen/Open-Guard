import { ComponentFixture, TestBed } from '@angular/core/testing';
import { ThreatsComponent } from './threats';
import { ThreatService } from '../core/services/threat.service';
import { of } from 'rxjs';

describe('ThreatsComponent', () => {
  let component: ThreatsComponent;
  let fixture: ComponentFixture<ThreatsComponent>;
  let mockThreatService: any;

  beforeEach(async () => {
    mockThreatService = {
      listAlerts: jasmine.createSpy('listAlerts').and.returnValue(of([])),
    };

    await TestBed.configureTestingModule({
      imports: [ThreatsComponent],
      providers: [
        { provide: ThreatService, useValue: mockThreatService }
      ]
    }).compileComponents();

    fixture = TestBed.createComponent(ThreatsComponent);
    component = fixture.componentInstance;
    fixture.detectChanges();
  });

  it('should create', () => {
    expect(component).toBeTruthy();
  });

  it('should call fetchThreats on init', () => {
    expect(mockThreatService.listAlerts).toHaveBeenCalled();
  });

  it('should update stats when alerts are loaded', () => {
    const mockAlerts = [
      { id: '1', severity: 'CRITICAL', status: 'OPEN', created_at: new Date().toISOString() },
      { id: '2', severity: 'LOW', status: 'RESOLVED', created_at: new Date().toISOString() }
    ];
    mockThreatService.listAlerts.and.returnValue(of(mockAlerts));
    
    component.fetchThreats();
    
    expect(component.stats().total).toBe(2);
    expect(component.stats().critical).toBe(1);
    expect(component.stats().open).toBe(1);
    expect(component.stats().resolved).toBe(1);
  });

  it('should update charts when alerts are loaded', () => {
    const mockAlerts = [
      { id: '1', severity: 'CRITICAL', status: 'OPEN', created_at: new Date().toISOString() }
    ];
    mockThreatService.listAlerts.and.returnValue(of(mockAlerts));
    
    // Spy on chart updates
    const severitySpy = spyOn(component.severityChart!, 'update');
    const trendSpy = spyOn(component.trendChart!, 'update');
    
    component.fetchThreats();
    
    expect(severitySpy).toHaveBeenCalled();
    expect(trendSpy).toHaveBeenCalled();
    
    // Verify chart data
    expect(component.severityChart?.data.datasets[0].data).toEqual([1, 0, 0, 0]);
  });
});
