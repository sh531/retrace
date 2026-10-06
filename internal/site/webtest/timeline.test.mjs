import assert from "node:assert/strict";
import { test } from "node:test";

import { load } from "./load.mjs";

const { buildTimeline, clockAt, lastPhotoReached, legSeconds, pauseStart, placeAt } = load("timeline.js");

test("legSeconds", () => {
  const cases = [
    { meters: 0, want: 0 }, // photos at the same place
    { meters: 50, want: 1.5 }, // the shortest leg
    { meters: 300, want: 3 }, // 100 m/s
    { meters: 5000, want: 8 }, // the longest leg
  ];
  for (const c of cases) assert.equal(legSeconds(c.meters), c.want, `${c.meters} m`);
});

// Photos at 300 m, without a time, at 300 m again, and at 1,100 m, on a
// track 1,400 m long.
const timeline = buildTimeline([300, undefined, 300, 1100], 1400);

test("buildTimeline", () => {
  assert.deepEqual(timeline.parts, [
    { start: 0, seconds: 3, from: 0, to: 300, stop: undefined },
    { start: 3, seconds: 3, from: 300, to: 300, stop: 0 },
    { start: 6, seconds: 0, from: 300, to: 300, stop: undefined }, // no move to a photo at the same place
    { start: 6, seconds: 3, from: 300, to: 300, stop: 2 },
    { start: 9, seconds: 8, from: 300, to: 1100, stop: undefined },
    { start: 17, seconds: 3, from: 1100, to: 1100, stop: 3 },
    { start: 20, seconds: 3, from: 1100, to: 1400, stop: undefined },
  ]);
  assert.equal(timeline.total, 23);
});

test("placeAt", () => {
  const cases = [
    { name: "start", seconds: 0, want: { index: 0, meters: 0 } },
    { name: "halfway through the first leg", seconds: 1.5, want: { index: 0, meters: 150 } },
    { name: "eased near the leg's start", seconds: 0.3, want: { index: 0, meters: 150 * (1 - Math.cos(Math.PI / 10)) } },
    { name: "arriving at a photo", seconds: 3, want: { index: 1, meters: 300 } }, // the pause, not the leg's end
    { name: "pausing at a photo", seconds: 4, want: { index: 1, meters: 300 } },
    { name: "the second photo there", seconds: 7, want: { index: 3, meters: 300 } },
    { name: "end", seconds: 23, want: { index: 6, meters: 1400 } },
    { name: "past the end", seconds: 99, want: { index: 6, meters: 1400 } },
  ];
  for (const c of cases) {
    const got = placeAt(timeline, c.seconds);
    assert.equal(got.index, c.want.index, `${c.name}: index`);
    assert.ok(Math.abs(got.meters - c.want.meters) < 1e-9, `${c.name}: meters got ${got.meters}, want ${c.want.meters}`);
  }
});

test("clockAt undoes placeAt", () => {
  for (const meters of [0, 1, 150, 299, 301, 700, 1099, 1250, 1400]) {
    const got = placeAt(timeline, clockAt(timeline, meters)).meters;
    assert.ok(Math.abs(got - meters) < 1e-6, `${meters} m: got ${got}`);
  }
  assert.equal(clockAt(timeline, -10), 0, "before the start");
  assert.equal(clockAt(timeline, 9999), 23, "past the end");
});

test("pauseStart", () => {
  const cases = [
    { stop: 0, want: 3 },
    { stop: 1, want: undefined }, // no time, so not on the timeline
    { stop: 3, want: 17 },
  ];
  for (const c of cases) assert.equal(pauseStart(timeline, c.stop), c.want, `photo ${c.stop}`);
});

test("lastPhotoReached", () => {
  const cases = [
    { seconds: 0, want: undefined },
    { seconds: 2.9, want: undefined },
    { seconds: 3, want: 0 },
    { seconds: 6, want: 2 },
    { seconds: 16.9, want: 2 },
    { seconds: 23, want: 3 },
  ];
  for (const c of cases) assert.equal(lastPhotoReached(timeline, c.seconds), c.want, `${c.seconds} s`);
});

test("buildTimeline without photos", () => {
  const empty = buildTimeline([], 1000);
  assert.deepEqual(empty.parts, [{ start: 0, seconds: 8, from: 0, to: 1000, stop: undefined }]);
  assert.equal(lastPhotoReached(empty, 5), undefined);
});
