// The flythrough's timeline for retrace's page: when the hiker moves along
// the drawn track and when it pauses at a photo. Every function takes the
// timeline it works on as an argument and touches no page state, so the
// tests in internal/site/webtest can run it in Node.
//
// A timeline is {parts, total}: parts in order, and their total length in
// seconds. Each part is {start, seconds, from, to, stop}: from and to are
// metres along the drawn track, and stop is the index of the photo a pause
// shows, undefined while moving.
"use strict";

const metersPerSecond = 100; // how fast the hiker travels, before clamping a leg
const minLegSeconds = 1.5;
const maxLegSeconds = 8; // a long stretch without photos isn't worth waiting for
const photoSeconds = 3; // how long each photo shows

// buildTimeline returns the timeline for photos at stopMeters along a drawn
// track trackMeters long, undefined for a photo without a time, which it
// skips. It moves to each photo, pauses there, and goes on to the track's end.
function buildTimeline(stopMeters, trackMeters) {
  const parts = [];
  let total = 0;
  const add = (seconds, from, to, stop) => {
    parts.push({ start: total, seconds, from, to, stop });
    total += seconds;
  };
  let at = 0;
  stopMeters.forEach((meters, stop) => {
    if (meters === undefined) return;
    add(legSeconds(meters - at), at, meters);
    add(photoSeconds, meters, meters, stop);
    at = meters;
  });
  add(legSeconds(trackMeters - at), at, trackMeters);
  return { parts, total };
}

// legSeconds returns how long the hiker takes to travel meters: none for
// photos at the same place.
function legSeconds(meters) {
  return meters > 0 ? Math.min(Math.max(meters / metersPerSecond, minLegSeconds), maxLegSeconds) : 0;
}

// ease starts and ends a leg slowly, so the hiker doesn't jolt at photos.
function ease(fraction) {
  return (1 - Math.cos(Math.PI * fraction)) / 2;
}

// placeAt returns where the flythrough is at seconds: index, the index in
// parts of the part playing, and meters, how far along the drawn track the
// hiker is.
function placeAt(timeline, seconds) {
  const index = Math.max(timeline.parts.findLastIndex((part) => part.start <= seconds), 0);
  const part = timeline.parts[index];
  const fraction = part.seconds > 0 ? Math.min((seconds - part.start) / part.seconds, 1) : 1;
  return { index, meters: part.from + ease(fraction) * (part.to - part.from) };
}

// clockAt returns the time when the hiker passes meters along the drawn
// track, on the way to the next photo.
function clockAt(timeline, meters) {
  const part = timeline.parts.find((p) => p.stop === undefined && p.to > p.from && meters <= p.to);
  if (!part) return meters <= 0 ? 0 : timeline.total;
  const fraction = Math.min(Math.max((meters - part.from) / (part.to - part.from), 0), 1);
  return part.start + (Math.acos(1 - 2 * fraction) / Math.PI) * part.seconds; // inverse of ease
}

// pauseStart returns when the pause at photo stop starts, or undefined if the
// photo isn't on the timeline.
function pauseStart(timeline, stop) {
  return timeline.parts.find((part) => part.stop === stop)?.start;
}

// lastPhotoReached returns the photo whose pause started last at or before
// seconds, or undefined before the first.
function lastPhotoReached(timeline, seconds) {
  return timeline.parts.findLast((part) => part.stop !== undefined && part.start <= seconds)?.stop;
}
