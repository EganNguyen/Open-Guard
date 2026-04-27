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
      { id: '1', severity: 'CRITICAL', status: 'OPEN' },
      { id: '2', severity: 'LOW', status: 'RESOLVED' }
    ];
    mockThreatService.listAlerts.and.returnValue(of(mockAlerts));
    
    component.fetchThreats();
    
    expect(component.stats().total).toBe(2);
    expect(component.stats().critical).toBe(1);
    expect(component.stats().open).toBe(1);
    expect(component.stats().resolved).toBe(1);
  });
});
