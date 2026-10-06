// Formatting for retrace's page: times, distances, elevations, and the text
// about a photo. Every function takes what it needs as arguments and touches
// no page state, so the tests in internal/site/webtest can run it in Node.
// Units are "metric" or "imperial"; an offset is minutes east of UTC, or
// undefined for the viewer's own timezone.
"use strict";

// firstRecordedOffset returns the first timezone any photo's camera recorded,
// in minutes east of UTC, or undefined if none did.
function firstRecordedOffset(photos) {
  return photos.find((p) => p.recorded_offset_minutes !== undefined)?.recorded_offset_minutes;
}

// formatTime formats an RFC 3339 time, or milliseconds since 1970, as the
// local time offsetMinutes east of UTC, or in the viewer's timezone if
// offsetMinutes is undefined.
function formatTime(time, offsetMinutes, options) {
  const date = new Date(time);
  if (offsetMinutes === undefined) return date.toLocaleString(undefined, options);
  // Shifted by the offset, the UTC fields are the local time there.
  return new Date(date.getTime() + offsetMinutes * 60_000).toLocaleString(undefined, { ...options, timeZone: "UTC" });
}

// formatOffset returns e.g. "UTC−07:00".
function formatOffset(minutes) {
  const abs = Math.abs(minutes);
  const hh = String(Math.floor(abs / 60)).padStart(2, "0");
  const mm = String(abs % 60).padStart(2, "0");
  return `UTC${minutes < 0 ? "−" : "+"}${hh}:${mm}`;
}

function formatElevation(meters, units) {
  const feet = units === "imperial";
  return `${Math.round(feet ? meters / 0.3048 : meters).toLocaleString()} ${feet ? "ft" : "m"}`;
}

function formatDistance(meters, units) {
  const value = units === "imperial" ? meters / 1609.344 : meters / 1000;
  return `${value.toFixed(1)} ${units === "imperial" ? "mi" : "km"}`;
}

// formatDuration returns e.g. "2 h 14 m", or "40 s" under a minute.
function formatDuration(seconds) {
  seconds = Math.round(Math.abs(seconds));
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  if (h > 0) return `${h} h ${m} m`;
  return m > 0 ? `${m} m` : `${seconds} s`;
}

// formatSettings returns a photo's camera settings, e.g. "78 mm · f/11 ·
// 1/250 s · ISO 200", leaving out those unknown.
function formatSettings(s) {
  if (!s) return "";
  const parts = [];
  if (s.focal_length_mm) parts.push(`${Math.round(s.focal_length_mm)} mm`);
  if (s.f_number) parts.push(`f/${s.f_number}`);
  if (s.exposure_time_seconds) {
    const t = s.exposure_time_seconds;
    parts.push(t < 1 ? `1/${Math.round(1 / t)} s` : `${t} s`);
  }
  if (s.iso) parts.push(`ISO ${s.iso}`);
  return parts.join(" · ");
}

// locationNote says how a photo's pin was placed.
function locationNote(location) {
  switch (location.source) {
    case "exif":
      return "Placed by the photo's GPS.";
    case "interpolated":
      return "Placed on the track by the time it was taken.";
    case "track_end": {
      const gap = formatDuration(location.seconds_from_track_end);
      return location.seconds_from_track_end < 0
        ? `Taken ${gap} before the track started; placed at its start.`
        : `Taken ${gap} after the track ended; placed at its end.`;
    }
  }
  return "";
}

// photoTime returns the date and time a photo was taken, with its timezone
// when known, e.g. "Oct 12, 2024, 11:30 AM (UTC−07:00)".
function photoTime(photo, offsetMinutes) {
  if (!photo.time) return "Time unknown";
  const time = formatTime(photo.time, offsetMinutes, { dateStyle: "medium", timeStyle: "short" });
  return offsetMinutes === undefined ? time : `${time} (${formatOffset(offsetMinutes)})`;
}

// photoLabel returns when a photo was taken and, if known, at what
// elevation, e.g. "10:29 AM · 2,280 m".
function photoLabel(photo, offsetMinutes, units) {
  const parts = [photo.time ? formatTime(photo.time, offsetMinutes, { timeStyle: "short" }) : "Time unknown"];
  const elevation = photo.location?.elevation_meters;
  if (elevation !== undefined) parts.push(formatElevation(elevation, units));
  return parts.join(" · ");
}

// placeLabel returns when the hiker was at a place on the drawn track, its
// elevation if known, and how far along the track it is, e.g. "10:31 AM ·
// 2,301 m · 12.6 km".
function placeLabel(at, offsetMinutes, units) {
  const parts = [formatTime(at.time, offsetMinutes, { timeStyle: "short" })];
  if (at.elevation !== undefined) parts.push(formatElevation(at.elevation, units));
  parts.push(formatDistance(at.meters, units));
  return parts.join(" · ");
}
