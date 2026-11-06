// SPDX-License-Identifier: Apache-2.0
// Every date on screen goes through here, so it follows a person's choice of time
// zone and format (spec 013, F7).
import { usePreferences } from './preferences';

const SECOND = 1000;
const MINUTE = 60 * SECOND;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;
const RELATIVE_DAYS = 30;
const ISO_LANGUAGE = 'sv-SE'; // The one locale that writes dates as 2026-11-05 13:45.

export const DATE_FORMATS = ['short', 'medium', 'long', 'iso'] as const;
export type DateFormat = (typeof DATE_FORMATS)[number];

export interface DateChoice {
  language: string;
  /** An IANA zone, or empty for the zone of this device. */
  timeZone: string;
  format: string;
}

/** A browser can know fewer zones than the server: such a zone cannot be used here. */
export function isKnownTimeZone(zone: string): boolean {
  try {
    new Intl.DateTimeFormat('en', { timeZone: zone });
    return true;
  } catch {
    return false;
  }
}

function zoneOf(choice: DateChoice): string | undefined {
  return choice.timeZone && isKnownTimeZone(choice.timeZone) ? choice.timeZone : undefined;
}

function styleOf(format: string): 'short' | 'medium' | 'long' {
  return format === 'short' || format === 'long' ? format : 'medium';
}

/** A day, such as "5 Nov 2026". */
export function dayText(value: string, choice: DateChoice): string {
  const timeZone = zoneOf(choice);
  if (choice.format === 'iso') {
    return new Intl.DateTimeFormat(ISO_LANGUAGE, {
      timeZone,
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    }).format(new Date(value));
  }
  return new Intl.DateTimeFormat(choice.language, { dateStyle: styleOf(choice.format), timeZone }).format(
    new Date(value),
  );
}

/** A day and a time. Nothing in, nothing out: many moments are optional. */
export function momentText(value: string | null | undefined, choice: DateChoice): string {
  if (!value) return '';
  const timeZone = zoneOf(choice);
  if (choice.format === 'iso') {
    return new Intl.DateTimeFormat(ISO_LANGUAGE, {
      timeZone,
      dateStyle: 'short',
      timeStyle: 'short',
    }).format(new Date(value));
  }
  return new Intl.DateTimeFormat(choice.language, {
    dateStyle: styleOf(choice.format),
    timeStyle: 'short',
    timeZone,
  }).format(new Date(value));
}

/** "3 hours ago" in the person's language. Older than a month is a date. */
export function relativeText(value: string, now: number, choice: DateChoice): string {
  const past = now - new Date(value).getTime();
  if (past >= RELATIVE_DAYS * DAY) return dayText(value, choice);
  const format = new Intl.RelativeTimeFormat(choice.language, { numeric: 'auto' });
  if (past >= DAY) return format.format(-Math.floor(past / DAY), 'day');
  if (past >= HOUR) return format.format(-Math.floor(past / HOUR), 'hour');
  if (past >= MINUTE) return format.format(-Math.floor(past / MINUTE), 'minute');
  return format.format(0, 'second');
}

/** The time right now in another person's zone, or nothing when the zone is unknown here. */
export function localTimeIn(zone: string, language: string, now = new Date()): string {
  if (!zone || !isKnownTimeZone(zone)) return '';
  return new Intl.DateTimeFormat(language, { timeZone: zone, timeStyle: 'short' }).format(now);
}

/** Dates in the way this person chose. */
export function useDates() {
  const { preferences } = usePreferences();
  const choice: DateChoice = {
    language: preferences.language,
    timeZone: preferences.time_zone,
    format: preferences.date_format,
  };
  return {
    language: preferences.language,
    moment: (value: string | null | undefined) => momentText(value, choice),
    day: (value: string) => dayText(value, choice),
    relative: (value: string) => relativeText(value, Date.now(), choice),
  };
}
