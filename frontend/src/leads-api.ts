export type LeadField = {
  key: string;
  label: string;
  labels: Record<string, string>;
  description: string;
  type: string;
  required: boolean;
  active: boolean;
  position: number;
  options: string[];
  min_length: number;
  max_length: number;
  min: number | null;
  max: number | null;
  integer: boolean;
};
export type LeadSettings = {
  enabled: boolean;
  revision: number;
  title: string;
  goal: string;
  require_contact: boolean;
  success_messages: Record<string, string>;
  fields: LeadField[];
};
export type LeadValue = {
  value: string | number;
  valid: boolean;
  error_code: string;
  source: string;
  message_id?: string;
  updated_at: string;
};
export type Collection = {
  state: string;
  version: number;
  conversation_version: number;
  values: Record<string, LeadValue>;
  fields: LeadField[];
  missing: string[];
  paused: boolean;
  enabled: boolean;
  privacy_url: string;
  error_code: string;
  lead_id?: string;
  demo: boolean;
};
export type Lead = {
  id: string;
  name: string;
  email: string;
  phone: string;
  site: string;
  conversation_id: string;
  status: string;
  assigned_user_id: string | null;
  version: number;
  values: Record<string, LeadValue>;
  snapshot: LeadSettings;
  created_at: string;
  source: string;
  privacy_url: string;
  can_open_conversation: boolean;
};
export type LeadDetail = {
  lead: Lead;
  events: {
    id: string;
    kind: string;
    status: string;
    actor_id: string | null;
    target_id: string | null;
    note: string;
    created_at: string;
  }[];
};
