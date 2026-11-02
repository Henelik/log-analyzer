export interface FileRecord {
  id: number;
  filename: string;
  headers: string;
  status: number;
  lines: number;
  accepted_lines: number;
  rejected_lines: number;
  duplicate_lines: number;
  created_at: string;
  updated_at: string;
}

export interface Detection {
  detection_id: number;
  description: string;
  severity: number;
  confidence: number;
  priority: number;
  entity: string;
  observed: number;
  threshold: number;
  events: number[];
  notes: string;
}

export interface EventError {
  line_number: number;
  raw_log: string;
  error: string;
}

export interface EventRecord {
  line_number: number;
  time: string | null;
  user: string;
  client_ip: string;
  server_ip: string;
  server_port: number | null;
  url: string;
  method: string;
  response_code: number | null;
  action: string;
  app_name: string;
  bytes_in: number | null;
  bytes_out: number | null;
  raw_log: string;
  status: number;
  errors: string;
}

export interface Session {
  token: string;
  username: string;
}

export interface UploadResult {
  id: number;
  filename: string;
  bytes: number;
  status: number;
}

export class ApiError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'ApiError';
    this.status = status;
  }
}

const sessionKey = 'log-analyzer.session';

export function getSession(): Session | null {
  const raw = localStorage.getItem(sessionKey);
  if (raw === null) {
    return null;
  }

  try {
    const parsed: unknown = JSON.parse(raw);
    if (typeof parsed !== 'object' || parsed === null) {
      return null;
    }

    const candidate = parsed as Partial<Session>;
    if (typeof candidate.token !== 'string' || typeof candidate.username !== 'string') {
      return null;
    }

    return { token: candidate.token, username: candidate.username };
  } catch {
    return null;
  }
}

export function saveSession(session: Session): void {
  localStorage.setItem(sessionKey, JSON.stringify(session));
}

export function clearSession(): void {
  localStorage.removeItem(sessionKey);
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  const session = getSession();

  if (session !== null) {
    headers.set('Authorization', `Bearer ${session.token}`);
  }

  const response = await fetch(path, { ...init, headers });

  if (!response.ok) {
    let message = `request failed with status ${response.status}`;

    try {
      const body: unknown = await response.json();
      if (typeof body === 'object' && body !== null && 'error' in body) {
        const candidate = (body as { error: unknown }).error;
        if (typeof candidate === 'string' && candidate !== '') {
          message = candidate;
        }
      }
    } catch {
      // The response body is not JSON; keep the status-based message.
    }

    throw new ApiError(message, response.status);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export async function login(username: string, password: string): Promise<string> {
  const body = new FormData();
  body.set('username', username);
  body.set('password', password);

  const payload = await request<{ token: string }>('/api/v1/login', { method: 'POST', body });

  return payload.token;
}

export async function listFiles(): Promise<FileRecord[]> {
  const payload = await request<{ files: FileRecord[] }>('/api/v1/files/');

  return payload.files;
}

export async function uploadFile(file: File): Promise<UploadResult> {
  const body = new FormData();
  body.set('file', file);

  return request<UploadResult>('/api/v1/files/', { method: 'POST', body });
}

export async function getDetections(id: number): Promise<Detection[]> {
  const payload = await request<{ detections: Detection[] }>(`/api/v1/files/${id}/detections`);

  return payload.detections;
}

export async function getEventErrors(id: number): Promise<EventError[]> {
  const payload = await request<{ errors: EventError[] }>(`/api/v1/files/${id}/errors`);

  return payload.errors;
}

export async function getEvents(id: number, lineNumbers: number[]): Promise<EventRecord[]> {
  const payload = await request<{ events: EventRecord[] }>(`/api/v1/files/${id}/events`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(lineNumbers)
  });

  return payload.events;
}
