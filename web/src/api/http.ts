// Typed HTTP client of the node API: every call goes through here.
import { textFor, type ErrorParams } from "../lib/errors";

// ApiError keeps what the backend sent: status, stable code, technical message
// (detail) and params. Its message is the Spanish text of the code, so what
// the components show as error.message is already the one the user reads.
export class ApiError extends Error {
  constructor(public status: number, public code: string, public detail: string, public params: ErrorParams = {}) {
    super(textFor(code, detail, params));
  }
}

export async function apiError(res: Response): Promise<ApiError> {
  const e = (await res.json().catch(() => null))?.error;
  return new ApiError(res.status, e?.code ?? "", e?.message ?? res.statusText, e?.params ?? {});
}

export async function req<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const res = await fetch(`/api${path}`, {
    method,
    signal,
    credentials: "include",
    headers: body === undefined ? undefined : { "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw await apiError(res);
  return res.status === 204 ? (undefined as T) : res.json();
}

export const get = <T>(path: string, signal?: AbortSignal) => req<T>("GET", path, undefined, signal);
export const post = <T>(path: string, body?: unknown) => req<T>("POST", path, body);
export const put = <T>(path: string, body?: unknown) => req<T>("PUT", path, body);
export const patch = <T>(path: string, body?: unknown) => req<T>("PATCH", path, body);
export const del = <T = void>(path: string) => req<T>("DELETE", path);

// upload sends a multipart form (attachments).
export async function upload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(`/api${path}`, { method: "POST", credentials: "include", body: form });
  if (!res.ok) throw await apiError(res);
  return res.json() as Promise<T>;
}

// tz is the browser time zone, sent with the usage reports.
export const tz = () => Intl.DateTimeFormat().resolvedOptions().timeZone;

const listQuery = (filter?: Record<string, unknown>, archived = false) =>
  `?limit=1000${filter ? `&filter=${encodeURIComponent(JSON.stringify(filter))}` : ""}${archived ? "&include_archived=true" : ""}`;
export { listQuery };

// Generic resource: list, get, create, update (fields left out keep their value) and archive.
export function resource<T extends { id: string }, I>(path: string) {
  return {
    list: (filter?: Record<string, unknown>, archived = false) => get<T[]>(path + listQuery(filter, archived)),
    get: (id: string) => get<T>(`${path}/${id}`),
    create: (b: I) => post<T>(path, b),
    update: (id: string, b: Partial<I>) => put<T>(`${path}/${id}`, b),
    archive: (id: string) => del(`${path}/${id}`),
  };
}
