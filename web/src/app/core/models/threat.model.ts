export interface ThreatAlert {
  id: string;
  org_id: string;
  user_id?: string;
  title: string;
  detector: string;
  detector_id: string;
  severity: 'LOW' | 'MEDIUM' | 'HIGH' | 'CRITICAL';
  status: 'OPEN' | 'ACKNOWLEDGED' | 'RESOLVED';
  score: number;
  metadata: Record<string, any>;
  created_at: string;
  updated_at: string;
}

export interface ThreatStats {
  open_alerts: number;
  high_severity_count: number;
  alerts_by_detector: Record<string, number>;
  trend: { date: string; count: number }[];
}
