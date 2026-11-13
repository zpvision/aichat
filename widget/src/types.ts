export type Locale = "ru" | "en" | "cs" | "de";
export const locales: Locale[] = ["ru", "en", "cs", "de"];
export type Copy = { title: string; subtitle: string; greeting: string };
export type Theme = {
  texts: Partial<Record<Locale, Copy>>;
  primary: string;
  secondary: string;
  user_color: string;
  launcher_color: string;
  position: "left" | "right";
  offset_x: number;
  offset_y: number;
  show_launcher: boolean;
  remember_session: boolean;
  allow_new: boolean;
  default_locale: Locale;
  auto_locale: boolean;
  privacy_url: string;
};
export type Config = {
  id: string;
  settings: Theme;
  logo_url: string;
  provider: string;
  demo: boolean;
  available: boolean;
  lead_enabled?: boolean;
  max_message_chars: number;
};
export type Session = { id: string; token: string; expires_at?: string };
export type Message = {
  id: string;
  request_id: string;
  role: "user" | "assistant" | "operator";
  sequence: number;
  change_seq: number;
  content: string;
  status: string;
  created_at: string;
};
export const uuid =
  /^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$/;
