const statusLabels: Record<number, string> = {
  0: 'invalid',
  1: 'uploaded',
  2: 'parsed',
  3: 'analyzed'
};

export type Tone = 'pending' | 'ready' | 'bad' | 'quiet';

export function statusLabel(status: number): string {
  return statusLabels[status] ?? 'unknown';
}

export function statusTone(status: number): Tone {
  switch (status) {
    case 1:
      return 'pending';
    case 2:
      return 'ready';
    case 3:
      return 'ready';
    case 0:
      return 'bad';
    default:
      return 'quiet';
  }
}

export function statusNote(status: number): string {
  switch (status) {
    case 1:
      return 'Queued for parsing. Detections and errors appear after the parser runs.';
    case 2:
      return 'Parsed. The analysis worker is still examining this ingest.';
    case 3:
      return 'Analysis complete.';
    case 0:
      return 'Rejected before processing.';
    default:
      return 'Status unknown.';
  }
}

export function formatDate(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) {
    return value;
  }

  return new Intl.DateTimeFormat(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short'
  }).format(date);
}

export function formatNumber(value: number): string {
  return new Intl.NumberFormat().format(value);
}

export function level(score: number): 'low' | 'moderate' | 'elevated' | 'high' {
  if (score >= 80) {
    return 'high';
  }
  if (score >= 55) {
    return 'elevated';
  }
  if (score >= 30) {
    return 'moderate';
  }

  return 'low';
}
