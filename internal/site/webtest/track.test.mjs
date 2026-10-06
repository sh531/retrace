import assert from "node:assert/strict";
import { test } from "node:test";

import { load } from "./load.mjs";

const { bearingBetween, distanceMeters, drawTrack, headingAt, pointAt, smooth, thin } = load("track.js");

const metersPerDegree = (6371008.8 * Math.PI) / 180; // along a meridian

// trackPoints returns recorded points stepSeconds apart, going north along a
// meridian 0.001° (about 111 m) at a time, with the given elevations (null
// for none).
function trackPoints(stepSeconds, elevations) {
  return elevations.map((e, i) => ({
    lat: 47 + i * 0.001,
    lon: -120,
    time: new Date(Date.UTC(2024, 0, 1, 0, 0, i * stepSeconds)).toISOString(),
    ...(e === null ? {} : { elevation_meters: e }),
  }));
}

test("distanceMeters", () => {
  const cases = [
    { name: "same point", a: { lat: 47, lon: -120 }, b: { lat: 47, lon: -120 }, want: 0 },
    { name: "1° along a meridian", a: { lat: 47, lon: -120 }, b: { lat: 48, lon: -120 }, want: metersPerDegree },
    { name: "1° along the equator", a: { lat: 0, lon: 10 }, b: { lat: 0, lon: 11 }, want: metersPerDegree },
  ];
  for (const c of cases) {
    assert.ok(Math.abs(distanceMeters(c.a, c.b) - c.want) < 1e-6, c.name);
  }
});

test("bearingBetween", () => {
  const from = { lat: 0, lon: 0 };
  const cases = [
    { name: "north", to: { lat: 1, lon: 0 }, want: 0 },
    { name: "east", to: { lat: 0, lon: 1 }, want: 90 },
    { name: "south", to: { lat: -1, lon: 0 }, want: 180 },
    { name: "west", to: { lat: 0, lon: -1 }, want: -90 },
  ];
  for (const c of cases) {
    assert.ok(Math.abs(bearingBetween(from, c.to) - c.want) < 1e-9, `${c.name}: got ${bearingBetween(from, c.to)}`);
  }
});

test("smooth", () => {
  const cases = [
    {
      // 20 s apart, each ±5 s window holds only its own point.
      name: "gaps stay unknown",
      points: trackPoints(20, [100, null, null, 130]),
      want: [100, undefined, undefined, 130],
    },
    {
      // 1 s apart, every window holds all three points.
      name: "averages only known elevations",
      points: trackPoints(1, [10, null, 30]),
      want: [20, 20, 20],
    },
    { name: "no elevations", points: trackPoints(1, [null, null]), want: [undefined, undefined] },
  ];
  for (const c of cases) {
    assert.deepEqual(
      smooth(c.points, 5).map((p) => p.elevation),
      c.want,
      c.name,
    );
  }
});

test("smooth averages positions within the window", () => {
  const got = smooth(trackPoints(1, [null, null, null]), 5);
  assert.ok(Math.abs(got[0].lat - 47.001) < 1e-12, `got ${got[0].lat}`);
  assert.deepEqual(
    got.map((p) => p.time),
    trackPoints(1, [null, null, null]).map((p) => Date.parse(p.time)),
  );
});

test("thin", () => {
  const near = (i, meters) => ({ lat: 47 + (i * meters) / metersPerDegree, lon: -120 });
  const line = [0, 1, 2, 3, 4, 5].map((i) => near(i, 1)); // 1 m apart
  assert.deepEqual(thin(line, 1.5), [line[0], line[2], line[4], line[5]], "at least 1.5 m apart, then the last");
  assert.deepEqual(thin([line[0]], 1.5), [line[0]], "one point");
});

test("drawTrack measures along the line", () => {
  const got = drawTrack(trackPoints(20, [100, 110, 120]));
  assert.equal(got.length, 3);
  assert.equal(got[0].meters, 0);
  for (let i = 1; i < got.length; i++) {
    assert.ok(Math.abs(got[i].meters - got[i - 1].meters - 0.001 * metersPerDegree) < 1e-6, `meters at ${i}`);
  }
});

test("pointAt", () => {
  // 20 s apart, so smoothing leaves each point where it is.
  const track = drawTrack(trackPoints(20, [0, 100, 300, null]));
  const [a, b, c, d] = track;
  const cases = [
    { name: "at a point", key: "meters", value: b.meters, want: { meters: b.meters, elevation: 100, time: b.time } },
    { name: "a quarter of a segment", key: "meters", value: b.meters + (c.meters - b.meters) / 4, want: { elevation: 150, time: b.time + 5000 } },
    { name: "by time", key: "time", value: b.time + 10000, want: { elevation: 200, meters: (b.meters + c.meters) / 2 } },
    { name: "before the start", key: "meters", value: -50, want: { meters: 0, elevation: 0, time: a.time } },
    { name: "after the end", key: "time", value: d.time + 1e6, want: { meters: d.meters, elevation: undefined } },
    { name: "next to a gap", key: "meters", value: (c.meters + d.meters) / 2, want: { elevation: undefined } },
  ];
  for (const c of cases) {
    const got = pointAt(track, c.key, c.value);
    for (const [field, want] of Object.entries(c.want)) {
      if (want === undefined) assert.equal(got[field], undefined, `${c.name}: ${field}`);
      else assert.ok(Math.abs(got[field] - want) < 1e-6, `${c.name}: ${field} got ${got[field]}, want ${want}`);
    }
  }
});

test("pointAt on a track of one point", () => {
  const track = drawTrack(trackPoints(1, [5]));
  assert.deepEqual(pointAt(track, "meters", 10), { lat: 47, lon: -120, time: track[0].time, meters: 0, elevation: 5 });
});

test("headingAt", () => {
  const track = drawTrack(trackPoints(20, [null, null, null, null])); // north, 333 m
  assert.ok(Math.abs(headingAt(track, 250)) < 1e-6, `north: got ${headingAt(track, 250)}`);
  assert.equal(headingAt(track, 0), undefined, "at the start");
});
