// Track geometry for retrace's page: distances and bearings between points,
// the drawn track, and places along it. Points have lat and lon in degrees.
// Every function takes the track it works on as an argument and touches no
// page state, so the tests in internal/site/webtest can run it in Node.
"use strict";

// lonLat returns a point with lat and lon as MapLibre's [lon, lat] array.
function lonLat(p) {
  return [p.lon, p.lat];
}

const earthRadiusMeters = 6371008.8;

// distanceMeters returns the great-circle distance between two points (haversine).
function distanceMeters(a, b) {
  const rad = Math.PI / 180;
  const dLat = (b.lat - a.lat) * rad;
  const dLon = (b.lon - a.lon) * rad;
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(a.lat * rad) * Math.cos(b.lat * rad) * Math.sin(dLon / 2) ** 2;
  return 2 * earthRadiusMeters * Math.asin(Math.sqrt(h));
}

// bearingBetween returns the initial direction from one point to another, in
// degrees clockwise from north.
function bearingBetween(from, to) {
  const rad = Math.PI / 180;
  const y = Math.sin((to.lon - from.lon) * rad) * Math.cos(to.lat * rad);
  const x = Math.cos(from.lat * rad) * Math.sin(to.lat * rad) - Math.sin(from.lat * rad) * Math.cos(to.lat * rad) * Math.cos((to.lon - from.lon) * rad);
  return Math.atan2(y, x) / rad;
}

// The drawn track. GPS jitter makes raw points zigzag, and knot where the
// hiker stood still, so each point is averaged over ±smoothingSeconds, then
// thinned to one every minStepMeters. The page data keeps every raw point.
const smoothingSeconds = 5;
const minStepMeters = 2;

// drawTrack returns the line the page draws for the recorded track points
// ({lat, lon, time, elevation_meters}): smoothed, thinned, and with each
// point's meters, its distance along the line. Each point is {lat, lon,
// time, elevation, meters}, with time in milliseconds.
function drawTrack(points) {
  const line = thin(smooth(points, smoothingSeconds), minStepMeters);
  line.forEach((p, i) => {
    p.meters = i === 0 ? 0 : line[i - 1].meters + distanceMeters(line[i - 1], p);
  });
  return line;
}

// smooth returns line with each point moved to the average position of the
// points within seconds of it. Each keeps its time, in milliseconds, and
// gets the average elevation of those points that have one, or undefined if
// none do.
function smooth(line, seconds) {
  const times = line.map((p) => Date.parse(p.time));
  const windowMillis = seconds * 1000;
  let lo = 0; // first point in the window
  let hi = 0; // first point after the window
  let lat = 0;
  let lon = 0;
  let elevation = 0; // sum over the points in the window that have one
  let withElevation = 0;
  return line.map((_, i) => {
    for (; hi < line.length && times[hi] <= times[i] + windowMillis; hi++) {
      lat += line[hi].lat;
      lon += line[hi].lon;
      if (line[hi].elevation_meters !== undefined) {
        elevation += line[hi].elevation_meters;
        withElevation++;
      }
    }
    for (; times[lo] < times[i] - windowMillis; lo++) {
      lat -= line[lo].lat;
      lon -= line[lo].lon;
      if (line[lo].elevation_meters !== undefined) {
        elevation -= line[lo].elevation_meters;
        withElevation--;
      }
    }
    return {
      lat: lat / (hi - lo),
      lon: lon / (hi - lo),
      time: times[i],
      elevation: withElevation > 0 ? elevation / withElevation : undefined,
    };
  });
}

// thin returns the first point, each point at least meters from the last one
// kept, and the last point.
function thin(line, meters) {
  const kept = [line[0]];
  for (const p of line) {
    if (distanceMeters(kept.at(-1), p) >= meters) kept.push(p);
  }
  if (kept.at(-1) !== line.at(-1)) kept.push(line.at(-1));
  return kept;
}

// pointAt returns the place on track, a drawn track, where key ("time" or
// "meters") reaches value, interpolated between its points and clamped to
// its ends: {lat, lon, time, meters, elevation}. Its elevation is undefined
// where either point around it has none.
function pointAt(track, key, value) {
  // Binary search for the last point at or before value, but not the last
  // point, so it starts a segment.
  let lo = 0;
  let hi = Math.max(track.length - 2, 0);
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (track[mid][key] <= value) lo = mid;
    else hi = mid - 1;
  }
  const a = track[lo];
  const b = track[lo + 1] ?? a; // a track of one point has no segment
  const span = b[key] - a[key];
  const fraction = span > 0 ? Math.min(Math.max((value - a[key]) / span, 0), 1) : 0;
  const between = (from, to) => from + fraction * (to - from);
  return {
    lat: between(a.lat, b.lat),
    lon: between(a.lon, b.lon),
    time: between(a.time, b.time),
    meters: between(a.meters, b.meters),
    elevation: a.elevation === undefined || b.elevation === undefined ? undefined : between(a.elevation, b.elevation),
  };
}

const lookBackMeters = 100;

// headingAt returns the direction the hiker was walking at meters along
// track, a drawn track, from the point lookBackMeters before, in degrees
// clockwise from north; undefined at the track's very start.
function headingAt(track, meters) {
  const from = pointAt(track, "meters", meters - lookBackMeters);
  const to = pointAt(track, "meters", meters);
  return distanceMeters(from, to) > 1 ? bearingBetween(from, to) : undefined;
}
