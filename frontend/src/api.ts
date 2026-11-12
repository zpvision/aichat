export class ApiError extends Error {
  constructor(
    public code: string,
    public status: number,
  ) {
    super(code);
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  data?: unknown,
): Promise<T> {
  let response: Response;
  try {
    response = await fetch("/api/v1" + path, {
      method,
      credentials: "same-origin",
      headers: { "Content-Type": "application/json", "X-CSRF-Protection": "1" },
      body: data === undefined ? undefined : JSON.stringify(data),
    });
  } catch {
    throw new ApiError("network", 0);
  }
  const body = await response.json().catch(() => null);
  if (!response.ok)
    throw new ApiError(body?.error?.code || "internal", response.status);
  return body as T;
}
export type Locale = "ru" | "en" | "cs" | "de";
export const locales: Locale[] = ["ru", "en", "cs", "de"];
export type User = {
  id: string;
  email: string;
  name: string;
  preferred_locale: Locale;
  created_at: string;
};
export type Organization = {
  id: string;
  name: string;
  country: string;
  timezone: string;
  default_locale: Locale;
  role: "owner" | "admin" | "operator";
  created_at: string;
};
export type Assistant = {
  id: string;
  name: string;
  description: string;
  default_locale: Locale;
  language_policy: string;
  status: string;
  created_at: string;
};
export type Site = {
  id: string;
  assistant_id: string;
  name: string;
  domain: string;
  status: string;
  created_at: string;
};
