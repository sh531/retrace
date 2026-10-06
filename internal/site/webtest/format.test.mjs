import assert from "node:assert/strict";
import { test } from "node:test";

import { load } from "./load.mjs";

const {
  firstRecordedOffset,
  formatDistance,
  formatDuration,
  formatElevation,
  formatOffset,
  formatSettings,
  formatTime,
  locationNote,
  photoLabel,
  photoTime,
  placeLabel,
} = load("format.js");

// Formatted times and numbers depend on the locale; make test-js sets LC_ALL.
test("locale is en-US", () => {
  assert.equal(Intl.DateTimeFormat().resolvedOptions().locale, "en-US", "run with LC_ALL=en_US.UTF-8, as make test-js does");
});

const pdt = -420; // UTC−07:00, minutes east of UTC

test("formatTime", () => {
  const cases = [
    { name: "in the recorded offset", time: "2024-10-12T18:30:00Z", offset: pdt, options: { timeStyle: "short" }, want: "11:30 AM" },
    { name: "across midnight", time: "2024-10-13T03:00:00Z", offset: pdt, options: { dateStyle: "medium" }, want: "Oct 12, 2024" },
    { name: "milliseconds", time: Date.parse("2024-10-12T18:30:00Z"), offset: 0, options: { timeStyle: "short" }, want: "6:30 PM" },
  ];
  for (const c of cases) assert.equal(formatTime(c.time, c.offset, c.options), c.want, c.name);
});

test("formatOffset", () => {
  const cases = [
    { minutes: pdt, want: "UTC−07:00" },
    { minutes: 330, want: "UTC+05:30" },
    { minutes: 0, want: "UTC+00:00" },
  ];
  for (const c of cases) assert.equal(formatOffset(c.minutes), c.want);
});

test("formatElevation and formatDistance", () => {
  const cases = [
    { got: formatElevation(2280, "metric"), want: "2,280 m" },
    { got: formatElevation(2280, "imperial"), want: "7,480 ft" },
    { got: formatDistance(32417, "metric"), want: "32.4 km" },
    { got: formatDistance(32417, "imperial"), want: "20.1 mi" },
  ];
  for (const c of cases) assert.equal(c.got, c.want);
});

test("formatDuration", () => {
  const cases = [
    { seconds: 40, want: "40 s" },
    { seconds: 600, want: "10 m" },
    { seconds: 3900, want: "1 h 5 m" },
    { seconds: 8040, want: "2 h 14 m" },
    { seconds: -1108, want: "18 m" }, // before the track: the gap, without its sign
  ];
  for (const c of cases) assert.equal(formatDuration(c.seconds), c.want);
});

test("formatSettings", () => {
  const cases = [
    { settings: { focal_length_mm: 78.4, f_number: 11, exposure_time_seconds: 0.004, iso: 200 }, want: "78 mm · f/11 · 1/250 s · ISO 200" },
    { settings: { exposure_time_seconds: 2 }, want: "2 s" },
    { settings: undefined, want: "" },
  ];
  for (const c of cases) assert.equal(formatSettings(c.settings), c.want);
});

test("locationNote", () => {
  const cases = [
    { location: { source: "exif" }, want: "Placed by the photo's GPS." },
    { location: { source: "interpolated" }, want: "Placed on the track by the time it was taken." },
    { location: { source: "track_end", seconds_from_track_end: -600 }, want: "Taken 10 m before the track started; placed at its start." },
    { location: { source: "track_end", seconds_from_track_end: 32400 }, want: "Taken 9 h 0 m after the track ended; placed at its end." },
  ];
  for (const c of cases) assert.equal(locationNote(c.location), c.want);
});

test("photo and place labels", () => {
  const photo = { time: "2024-10-12T18:30:56Z", location: { elevation_meters: 2296 } };
  const cases = [
    { name: "photoTime", got: photoTime(photo, pdt), want: "Oct 12, 2024, 11:30 AM (UTC−07:00)" },
    { name: "photoTime without a time", got: photoTime({}, pdt), want: "Time unknown" },
    { name: "photoLabel", got: photoLabel(photo, pdt, "metric"), want: "11:30 AM · 2,296 m" },
    { name: "photoLabel without an elevation", got: photoLabel({ time: photo.time }, pdt, "metric"), want: "11:30 AM" },
    { name: "placeLabel", got: placeLabel({ time: Date.parse(photo.time), elevation: 2296, meters: 11900 }, pdt, "imperial"), want: "11:30 AM · 7,533 ft · 7.4 mi" },
  ];
  for (const c of cases) assert.equal(c.got, c.want, c.name);
});

test("firstRecordedOffset", () => {
  assert.equal(firstRecordedOffset([{}, { recorded_offset_minutes: pdt }, { recorded_offset_minutes: 60 }]), pdt);
  assert.equal(firstRecordedOffset([{}]), undefined);
});
