export type Severity = "low" | "medium" | "high";

export interface EventT {
  id: string;
  event_type: string;
  principal: string;
  target: string;
  network: string;
  severity_hint: string;
  labels?: string[];
  description: string;
  ts: string;
}

export interface TriageT {
  severity: Severity;
  confidence: number;
  reason_tokens?: string[];
}

export interface AlertT {
  alert_id: string;
  event: EventT;
  triage: TriageT;
  status: "pending" | "awaiting_approval" | "action_executed" | "reviewed";
  created: string;
}

export interface AlertsResponse {
  alerts: AlertT[];
}
