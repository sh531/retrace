// retrace's page: the track and photos from the data inlined in index.html,
// on a MapLibre map with a list of photos beside it. It's a classic script,
// not a module, because browsers block module scripts in a page opened from
// a file.
//
// Its sections, in order: settings, formatting, track geometry, page
// elements, photos and the flythrough's timeline, the map, choosing a photo,
// the bar, the panel, flythrough playback, photo size, the header, and Start,
// which wires up the controls and draws the page. Everything above Start only
// defines things, or computes them from the data.
"use strict";

// The page's data, inlined in index.html by retrace.
const data = JSON.parse(document.getElementById("data").textContent);
const points = data.track.points;

// Settings the viewer chooses are kept in the browser between visits.

// loadSetting returns the saved value for key if it is one of allowed, else fallback.
function loadSetting(key, allowed, fallback) {
  try {
    const value = localStorage.getItem(key);
    return allowed.includes(value) ? value : fallback;
  } catch {
    return fallback; // storage blocked, e.g. in a private window
  }
}

function saveSetting(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // storage blocked: the choice lasts until the page is closed
  }
}

// Units
const unitsKey = "retrace.units";

// Countries that use miles: the US, Liberia, and Myanmar.
const imperialRegions = ["US", "LR", "MM"];

function defaultUnits() {
  const region = new Intl.Locale(navigator.language).maximize().region;
  return imperialRegions.includes(region) ? "imperial" : "metric";
}

let units = loadSetting(unitsKey, ["metric", "imperial"], defaultUnits());

// Photo size: the width of the photo in the panel and the popup.
const photoSizeKey = "retrace.photoSize";
const photoWidths = { s: 320, m: 480, l: 720 };

let photoSize = loadSetting(photoSizeKey, Object.keys(photoWidths), "m");

// Terrain: whether the map was last shown in 3D; read when terrain is added.
const terrainKey = "retrace.terrain";

// Formatting

// Times are shown as they were where the photos were taken: in the timezone
// each photo's camera recorded, or else the first one any camera recorded,
// which the track's date uses too. Only with none at all is the viewer's
// own timezone used.
const hikeOffsetMinutes = data.photos.find((p) => p.recorded_offset_minutes !== undefined)?.recorded_offset_minutes;

function offsetMinutes(photo) {
  return photo.recorded_offset_minutes ?? hikeOffsetMinutes;
}

// formatTime formats an RFC 3339 time as the local time offsetMinutes east of
// UTC, or in the viewer's timezone if offsetMinutes is undefined.
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

function formatElevation(meters) {
  const feet = units === "imperial";
  return `${Math.round(feet ? meters / 0.3048 : meters).toLocaleString()} ${feet ? "ft" : "m"}`;
}

// photoLabel returns when a photo was taken and, if known, at what
// elevation, e.g. "10:29 AM · 2,280 m".
function photoLabel(photo) {
  const parts = [photo.time ? formatTime(photo.time, offsetMinutes(photo), { timeStyle: "short" }) : "Time unknown"];
  const elevation = photo.location?.elevation_meters;
  if (elevation !== undefined) parts.push(formatElevation(elevation));
  return parts.join(" · ");
}

function formatDistance(meters) {
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

// Geometry

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

// Each drawn point also gets meters, its distance along the drawn line.
const drawnTrack = thin(smooth(points, smoothingSeconds), minStepMeters);
drawnTrack.forEach((p, i) => {
  p.meters = i === 0 ? 0 : drawnTrack[i - 1].meters + distanceMeters(drawnTrack[i - 1], p);
});
const trackMeters = drawnTrack.at(-1).meters;
const trackSeconds = (Date.parse(points.at(-1).time) - Date.parse(points[0].time)) / 1000;

// pointAt returns the place on the drawn track where key ("time" or
// "meters") reaches value, interpolated between drawn points and clamped to
// the track's ends: {lat, lon, time, meters, elevation}. Its elevation is
// undefined where either drawn point around it has none.
function pointAt(key, value) {
  // Binary search for the last drawn point at or before value, but not the
  // last point, so it starts a segment.
  let lo = 0;
  let hi = Math.max(drawnTrack.length - 2, 0);
  while (lo < hi) {
    const mid = Math.ceil((lo + hi) / 2);
    if (drawnTrack[mid][key] <= value) lo = mid;
    else hi = mid - 1;
  }
  const a = drawnTrack[lo];
  const b = drawnTrack[lo + 1] ?? a; // a track of one point has no segment
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

// The 3D camera looks the way the hiker was walking, so ridges ahead don't
// hide the pins: facing north, they hid those in the Colchuck Lake basin.
const lookBackMeters = 100;

// headingAt returns the direction the hiker was walking at meters along the
// drawn track, from the point lookBackMeters before, in degrees clockwise
// from north; undefined at the track's very start.
function headingAt(meters) {
  const from = pointAt("meters", meters - lookBackMeters);
  const to = pointAt("meters", meters);
  return distanceMeters(from, to) > 1 ? bearingBetween(from, to) : undefined;
}

// Page elements

// element returns a new element with a class and text.
function element(tag, className, text) {
  const el = document.createElement(tag);
  if (className) el.className = className;
  if (text) el.textContent = text;
  return el;
}

function photoURL(photo) {
  return `photos/${encodeURIComponent(photo.file)}`;
}

// photoTime returns the date and time a photo was taken, with its timezone when known.
function photoTime(photo) {
  if (!photo.time) return "Time unknown";
  const offset = offsetMinutes(photo);
  const time = formatTime(photo.time, offset, { dateStyle: "medium", timeStyle: "short" });
  return offset === undefined ? time : `${time} (${formatOffset(offset)})`;
}

// popupContent returns the popup for a photo: the photo, linked to the full
// size, then its details.
function popupContent(photo) {
  const box = element("div", "popup");
  const link = element("a");
  link.href = photoURL(photo);
  link.target = "_blank";
  const img = element("img");
  img.src = photoURL(photo);
  img.alt = photo.file;
  link.append(img);
  box.append(closeButton(), link, ...photoDetails(photo));
  return box;
}

// closeButton returns a new × that closes the photo, the same in the popup
// and the panel.
function closeButton() {
  const button = document.getElementById("close-button").content.firstElementChild.cloneNode(true);
  button.addEventListener("click", () => closePhoto());
  return button;
}

// photoDetails returns the lines under a photo in the popup and the panel:
// when it was taken, with what, and how its pin was placed.
function photoDetails(photo) {
  const lines = [element("p", "strong", photoTime(photo))];
  const camera = [photo.camera?.make, photo.camera?.model].filter(Boolean).join(" ");
  for (const line of [camera, photo.lens, formatSettings(photo.settings)]) {
    if (line) lines.push(element("p", "muted", line));
  }
  if (photo.location) lines.push(element("p", `note note-${photo.location.source}`, locationNote(photo.location)));
  return lines;
}

// listItem returns a photo's row in the list.
const rowLabels = []; // [photo, element] for each row, relabelled when the units change

function listItem(photo) {
  const li = element("li");
  const button = element("button");
  button.type = "button";
  const img = element("img");
  img.src = photoURL(photo);
  img.alt = "";
  img.loading = "lazy";
  img.decoding = "async";
  const text = element("span", "text");
  const label = element("span", "strong", photoLabel(photo));
  rowLabels.push([photo, label]);
  text.append(label);
  text.append(element("span", "muted", photo.file));
  button.append(img, text);
  if (photo.location) button.prepend(element("span", `pin pin-${photo.location.source}`));
  li.append(button);
  return li;
}

// Photos: a pin on the map and a row in the list for each located photo, and
// a row under "Not on the map" for the rest. The located photos, in time
// order, are the bar's stops; the arrows step through them.
const stops = []; // {photo, row, meters} for each located photo
let current = -1; // index in stops of the photo last shown; -1 before any

for (const photo of data.photos) {
  const row = listItem(photo);
  if (!photo.location) {
    row.querySelector("button").addEventListener("click", () => window.open(photoURL(photo), "_blank"));
    document.querySelector("#unlocated ol").append(row);
    document.getElementById("unlocated").hidden = false;
    continue;
  }

  const index = stops.length;
  // Where the hiker was on the drawn track when the photo was taken, which
  // is near its pin unless the pin came from the photo's GPS; undefined
  // without a time, so playback skips it.
  const meters = photo.time ? pointAt("time", Date.parse(photo.time)).meters : undefined;
  stops.push({ photo, row, meters });

  row.querySelector("button").addEventListener("click", () => show(index));
  row.addEventListener("mouseenter", () => highlightPin(index, true));
  row.addEventListener("mouseleave", () => highlightPin(index, false));
  document.getElementById("photos").append(row);
}

// Flythrough: ▶ moves a marker along the drawn track, pausing at each photo
// with a time, which shows in a panel above the bar. The timeline is a list
// of parts, each moving the marker from one place to the next or pausing at
// a photo; clock is the playback time in seconds.
const metersPerSecond = 100; // how fast the marker travels, before clamping a leg
const minLegSeconds = 1.5;
const maxLegSeconds = 8; // a long stretch without photos isn't worth waiting for
const photoSeconds = 3; // how long each photo shows
const turnSeconds = 1.5; // how slowly the 3D camera turns to the new bearing, so switchbacks don't swing it

// buildTimeline returns the flythrough's parts in order, and their total
// length in seconds. Each part is {start, seconds, from, to, stop}: from and
// to are metres along the drawn track, and stop is the index in stops of the
// photo a pause shows, undefined while moving. The parts move to each photo
// with a time, pause there, and go on to the track's end.
function buildTimeline() {
  const parts = [];
  let total = 0;
  const add = (seconds, from, to, stop) => {
    parts.push({ start: total, seconds, from, to, stop });
    total += seconds;
  };
  let at = 0;
  stops.forEach((stop, index) => {
    if (stop.meters === undefined) return;
    add(legSeconds(stop.meters - at), at, stop.meters);
    add(photoSeconds, stop.meters, stop.meters, index);
    at = stop.meters;
  });
  add(legSeconds(trackMeters - at), at, trackMeters);
  return { parts, total };
}

// legSeconds returns how long the marker takes to travel meters: none for
// photos at the same place.
function legSeconds(meters) {
  return meters > 0 ? Math.min(Math.max(meters / metersPerSecond, minLegSeconds), maxLegSeconds) : 0;
}

const { parts: timeline, total: timelineSeconds } = buildTimeline();

// Flythrough state
let clock = 0;
let playing = false;
let flying = false; // the hiker and the panel show, in place of the popup
let hiker = pointAt("meters", 0); // where the flythrough's marker is on the drawn track
let atPhoto = false; // whether the marker is pausing at a photo
let cameraBearing = 0;
let lastFrame;
let frameRequest; // the pending requestAnimationFrame, so only one loop runs

// Map

// cssColor returns the value of a colour variable in style.css, so the map's
// layers match the page.
function cssColor(name) {
  return getComputedStyle(document.documentElement).getPropertyValue(name).trim();
}

const tiltTip = "Ctrl-drag (or right-drag) to tilt and turn the map";
const map = new maplibregl.Map({
  container: "map",
  style: "https://tiles.openfreemap.org/styles/liberty",
  bounds: bounds(),
  fitBoundsOptions: { padding: 48 },
  keyboard: false, // the arrow keys step through the photos instead of panning
  maxPitch: 85,
  // The 3D button's tooltip also says how to tilt the map by hand.
  locale: {
    "TerrainControl.Enable": `Enable terrain\n${tiltTip}`,
    "TerrainControl.Disable": `Disable terrain\n${tiltTip}`,
  },
});
map.addControl(new maplibregl.NavigationControl({ visualizePitch: true }));
const scale = new maplibregl.ScaleControl({ unit: units });
map.addControl(scale);

// bounds returns the box around the track and every located photo.
function bounds() {
  const box = new maplibregl.LngLatBounds();
  for (const p of points) box.extend(lonLat(p));
  for (const photo of data.photos) {
    if (photo.location) box.extend(lonLat(photo.location));
  }
  return box;
}

map.on("load", () => {
  map.addSource("track", {
    type: "geojson",
    data: { type: "Feature", geometry: { type: "LineString", coordinates: drawnTrack.map(lonLat) } },
  });
  map.addLayer({
    id: "track",
    type: "line",
    source: "track",
    layout: { "line-join": "round", "line-cap": "round" },
    paint: { "line-color": cssColor("--track"), "line-width": 3 },
  });
  addPinLayers();
  addTerrain();
});

// Camera
const photoZoom = 14; // the map zooms in at least this far to show a photo or follow the hiker
const noPadding = { top: 0, bottom: 0, left: 0, right: 0 }; // undoes dockPadding's

// dockPadding returns map padding that keeps the centre clear of the dock:
// right of the photo panel, whether or not it shows yet, so the camera
// doesn't jump when it appears, and above the bar.
function dockPadding() {
  const box = map.getContainer().getBoundingClientRect();
  const dock = document.querySelector(".dock").getBoundingClientRect();
  return {
    ...noPadding,
    left: dock.left - box.left + Math.min(photoWidths[photoSize], dock.width),
    bottom: box.bottom - document.querySelector(".bar").getBoundingClientRect().top,
  };
}

// Terrain: a 3D button draws the map over Mapterhorn's elevation tiles, with
// hillshading. The page starts in 2D, which downloads no terrain tiles,
// unless the viewer chose 3D before.
const terrainTiles = "https://tiles.mapterhorn.com/tilejson.json";
const tiltDegrees = 60; // a top-down view of terrain looks flat, so 3D tilts the map

function addTerrain() {
  map.addSource("terrain", { type: "raster-dem", url: terrainTiles });
  // A separate source for hillshading, as MapLibre recommends, so the two
  // don't compete for the same cached tiles.
  map.addSource("hillshade", { type: "raster-dem", url: terrainTiles });
  const firstLine = map.getStyle().layers.find((layer) => layer.type === "line").id;
  map.addLayer(
    { id: "hillshade", type: "hillshade", source: "hillshade", layout: { visibility: "none" }, paint: { "hillshade-exaggeration": 0.3 } },
    firstLine, // below roads, paths, and the track
  );

  const terrain = { source: "terrain", exaggeration: 1 };
  map.addControl(new maplibregl.TerrainControl(terrain));
  map.on("terrain", () => {
    const on = !!map.getTerrain();
    map.setLayoutProperty("hillshade", "visibility", on ? "visible" : "none");
    if (on && map.getPitch() === 0) map.easeTo({ pitch: tiltDegrees });
    if (!on) map.easeTo({ pitch: 0 });
    saveSetting(terrainKey, on ? "3d" : "2d");
  });
  if (loadSetting(terrainKey, ["2d", "3d"], "2d") === "3d") map.setTerrain(terrain);
}

// Pins are circle layers rather than DOM markers. In 3D, MapLibre checks
// whether terrain hides each DOM marker on every camera move by reading a
// pixel back from the GPU, which held a moving camera to about 20 frames a
// second with 57 pins. Each pin is two circles, matching the legend's CSS
// pins: the pin with its white ring, over a slightly larger, faint dark one.
function addPinLayers() {
  const features = stops.map(({ photo }, index) => ({
    type: "Feature",
    id: index, // for highlightPin's feature state
    properties: { index, source: photo.location.source },
    geometry: { type: "Point", coordinates: lonLat(photo.location) },
  }));
  map.addSource("pins", { type: "geojson", data: { type: "FeatureCollection", features } });
  map.addSource("hiker", { type: "geojson", data: hikerGeoJSON() });

  // A circle's stroke is drawn outside its radius, so each radius is half
  // the CSS pin's size, 14 px or 20 px while its row is hovered, less the
  // stroke.
  const active = (yes, no) => ["case", ["boolean", ["feature-state", "active"], false], yes, no];
  const hollow = (yes, no) => ["match", ["get", "source"], "track_end", yes, no]; // before or after the track
  const shadow = "rgba(0, 0, 0, 0.3)";
  // By default a circle shrinks with its distance from the camera, so pins
  // shrank as 3D tilted the map and were small in the distance. "viewport"
  // keeps every pin its CSS size, as the legend's are.
  const fixedSize = { "circle-pitch-scale": "viewport" };
  map.addLayer({ id: "pin-shadows", type: "circle", source: "pins", paint: { ...fixedSize, "circle-radius": active(11, 8), "circle-color": shadow } });
  map.addLayer({
    id: "pins",
    type: "circle",
    source: "pins",
    paint: {
      ...fixedSize,
      "circle-radius": ["-", active(10, 7), hollow(3, 2)],
      "circle-stroke-width": hollow(3, 2),
      "circle-color": ["match", ["get", "source"], "exif", cssColor("--exif"), "interpolated", cssColor("--track"), "#fff"],
      "circle-stroke-color": hollow(cssColor("--track"), "#fff"),
    },
  });
  // The flythrough's marker, 18 px, shown while flying.
  const visibility = flying ? "visible" : "none";
  map.addLayer({ id: "hiker-shadow", type: "circle", source: "hiker", layout: { visibility }, paint: { ...fixedSize, "circle-radius": 10, "circle-color": shadow, "circle-blur": 0.3 } });
  map.addLayer({
    id: "hiker",
    type: "circle",
    source: "hiker",
    layout: { visibility },
    paint: { ...fixedSize, "circle-radius": 6, "circle-stroke-width": 3, "circle-color": cssColor("--text"), "circle-stroke-color": "#fff" },
  });

  // A click on a pin opens its photo where it is; anywhere else on the map
  // closes the popup.
  map.on("click", (event) => {
    const [pin] = map.queryRenderedFeatures(event.point, { layers: ["pins"] });
    if (pin) openPopup(pin.properties.index);
    else popup.remove();
  });
  map.on("mouseenter", "pins", () => (map.getCanvas().style.cursor = "pointer"));
  map.on("mouseleave", "pins", () => (map.getCanvas().style.cursor = ""));
}

function hikerGeoJSON() {
  return { type: "Point", coordinates: lonLat(hiker) };
}

// highlightPin enlarges the pin of the photo at index in stops, or returns
// it to its size.
function highlightPin(index, on) {
  if (map.getSource("pins")) map.setFeatureState({ source: "pins", id: index }, { active: on }); // added on load
}

// One popup shows the photo last opened. Always above the pin: MapLibre
// would put a tall photo below it. The map's click handler closes it, as
// MapLibre's own closeOnClick would close it again right after a pin's
// click opened it. Its × is the panel's, not MapLibre's.
const popup = new maplibregl.Popup({ offset: 12, maxWidth: "none", anchor: "bottom", closeOnClick: false, closeButton: false });
popup.on("close", () => stops[current]?.row.classList.remove("current"));

// resumeFrom sets the flythrough to resume from the photo at index in stops,
// and moves the profile's playhead to it. A photo without a time isn't on
// the timeline, so nothing changes.
function resumeFrom(index) {
  const { meters } = stops[index];
  if (meters === undefined) return;
  clock = timeline.find((part) => part.stop === index).start;
  renderPlayhead(meters);
}

// openPopup opens the popup of the photo at index in stops, leaving the
// flythrough, which resumes from this photo. The content is built here: an
// <img> starts downloading as soon as it has a src.
function openPopup(index) {
  leaveFlythrough();
  resumeFrom(index);
  const { photo } = stops[index];
  const content = popupContent(photo);
  content.querySelector("img").addEventListener("load", fitPopup);
  popup.remove(); // before select, as closing unhighlights the current row
  select(index);
  popup.setLngLat(lonLat(photo.location)).setDOMContent(content).addTo(map);
  fitPopup();
}

// show flies to the photo at index in stops and opens its popup.
function show(index) {
  const { photo, meters } = stops[index];
  const center = lonLat(photo.location);
  leaveFlythrough(); // before flyTo, so the follow camera doesn't move the map
  popup.remove();
  select(index);
  resumeFrom(index); // now, rather than once the camera arrives
  // No padding: the flythrough leaves some to keep its marker clear of the dock.
  const camera = { center, zoom: Math.max(map.getZoom(), photoZoom), padding: noPadding };
  if (map.getTerrain() && meters !== undefined) {
    // Look the way the hiker was walking, so the camera is over ground they
    // had crossed rather than behind the next ridge.
    camera.bearing = headingAt(meters) ?? map.getBearing();
  }
  map.flyTo(camera); // fitPopup makes room for the popup once it opens
  map.once("moveend", () => {
    if (current !== index) return; // another photo was chosen on the way
    // flyTo guesses the ground height at the destination before its terrain
    // has loaded, so it can land off the pin; centre on it again.
    if (map.getTerrain()) map.jumpTo({ center });
    openPopup(index);
  });
}

// fitPopup moves the map down if the popup reaches above its top edge, so
// all of a tall photo shows. It waits a frame: the popup's size is known only
// once it's on the page.
function fitPopup() {
  requestAnimationFrame(() => {
    if (!popup.isOpen()) return;
    const margin = 8;
    const overflow = map.getContainer().getBoundingClientRect().top + margin - popup.getElement().getBoundingClientRect().top;
    if (overflow > 0) map.panBy([0, -overflow]);
  });
}

// select makes the photo at index the current one, highlighting its row.
function select(index) {
  current = index;
  stops.forEach((stop, i) => stop.row.classList.toggle("current", i === index));
  stops[index].row.scrollIntoView({ block: "nearest" });
  renderBar();
}

// closePhoto leaves the flythrough, or closes the popup, back to the map.
function closePhoto() {
  leaveFlythrough();
  popup.remove();
  stops[current]?.row.classList.remove("current");
}

// Bar: the elevation profile and controls at the bottom of the map.
const previousButton = document.getElementById("previous");
const playButton = document.getElementById("play");
const nextButton = document.getElementById("next");
const barLabel = document.getElementById("bar-label");
const scrub = document.getElementById("scrub");

function renderBar() {
  if (flying && !atPhoto) {
    barLabel.textContent = placeLabel(hiker);
  } else if (current < 0) {
    barLabel.textContent = `${stops.length} photos on the map`;
  } else {
    barLabel.textContent = `${current + 1} / ${stops.length} · ${photoLabel(stops[current].photo)}`;
  }
  // The ends don't wrap: a hike has a start and an end.
  previousButton.disabled = current <= 0;
  nextButton.disabled = current >= stops.length - 1;
}

// placeLabel returns when the hiker was at a place on the drawn track, its
// elevation if known, and how far along the track it is, e.g. "10:31 AM ·
// 2,301 m · 12.6 km".
function placeLabel(at) {
  const parts = [formatTime(at.time, hikeOffsetMinutes, { timeStyle: "short" })];
  if (at.elevation !== undefined) parts.push(formatElevation(at.elevation));
  parts.push(formatDistance(at.meters));
  return parts.join(" · ");
}

// step shows the photo by stops after the current one; before any, → shows
// the first. It stops at the ends.
function step(by) {
  const index = current < 0 ? (by > 0 ? 0 : -1) : current + by;
  if (index >= 0 && index < stops.length) show(index);
}

// Elevation profile: distance along the drawn track against elevation, with
// a tick for each photo and a playhead. Its viewBox is in metres, so the SVG
// stretches to the bar's width without being redrawn; the strokes keep their
// width with vector-effect.
const profileSamples = 1000; // points drawn across the profile, about one per pixel
const profile = document.querySelector(".profile svg");
const playhead = profile.querySelector(".playhead");
const handle = document.querySelector(".profile .handle");

function renderProfile() {
  // Runs of drawn points with an elevation, thinned to profileSamples over
  // the track; a point without one ends a run, leaving a gap.
  const runs = [];
  let run;
  for (const p of drawnTrack) {
    if (p.elevation === undefined) run = undefined;
    else if (!run) runs.push((run = [p]));
    else if (p.meters - run.at(-1).meters >= trackMeters / profileSamples) run.push(p);
  }
  const elevations = runs.flat().map((p) => p.elevation);
  const lo = Math.min(...elevations);
  const hi = Math.max(...elevations);
  const span = Math.max(hi - lo, 1); // a flat track still gets a height
  const height = span * 1.15; // room above the highest point
  const width = Math.max(trackMeters, 1);
  profile.setAttribute("viewBox", `0 0 ${width} ${height}`);

  const y = (elevation) => (height - (elevation - lo)).toFixed(1);
  const line = runs.map((r) => "M" + r.map((p) => `${p.meters.toFixed(1)} ${y(p.elevation)}`).join("L"));
  const ground = runs.map((r, i) => `${line[i]}V${height}H${r[0].meters.toFixed(1)}Z`);
  profile.querySelector(".line").setAttribute("d", line.join(""));
  profile.querySelector(".ground").setAttribute("d", ground.join(""));

  // Ticks rise from the bottom, a quarter of the height.
  for (const source of ["exif", "interpolated", "track_end"]) {
    const ticks = stops.filter((stop) => stop.meters !== undefined && stop.photo.location.source === source);
    const d = ticks.map((stop) => `M${stop.meters.toFixed(1)} ${height}v${-height / 4}`).join("");
    profile.querySelector(`.ticks-${source}`).setAttribute("d", d);
  }
  scrub.max = String(trackMeters);
}

// Panel: the photo while flying, cross-faded from the one before. Two
// stacked images take turns; the new one fades in once it has loaded.
const panel = document.getElementById("panel");

function showPanel(photo) {
  panel.hidden = false;
  panel.querySelector("figcaption").replaceChildren(...photoDetails(photo));
  const [first, second] = panel.querySelectorAll("img");
  const [shown, hidden] = first.classList.contains("shown") ? [first, second] : [second, first];
  if (shown.getAttribute("src") === photoURL(photo)) return;
  // onload, not addEventListener: a newer photo replaces the handler of one
  // still loading.
  hidden.onload = () => {
    hidden.classList.add("shown");
    shown.classList.remove("shown");
  };
  hidden.src = photoURL(photo);
  hidden.alt = photo.file;
}

// preload starts downloading photo, so it can show as soon as the marker
// reaches it.
const preloaded = new Set();

function preload(photo) {
  if (!photo || preloaded.has(photo)) return;
  preloaded.add(photo);
  new Image().src = photoURL(photo);
}

// Flythrough playback

// showHiker shows or hides the flythrough's marker.
function showHiker(on) {
  if (!map.getLayer("hiker")) return; // not loaded yet; added visible if flying by then
  for (const id of ["hiker-shadow", "hiker"]) map.setLayoutProperty(id, "visibility", on ? "visible" : "none");
}

// ease starts and ends a leg slowly, so the marker doesn't jolt at photos.
function ease(fraction) {
  return (1 - Math.cos(Math.PI * fraction)) / 2;
}

// partIndexAt returns the index in timeline of the part playing at seconds.
function partIndexAt(seconds) {
  return Math.max(timeline.findLastIndex((part) => part.start <= seconds), 0);
}

// clockAt returns the playback time when the marker passes meters along the
// drawn track, on the way to the next photo.
function clockAt(meters) {
  const part = timeline.find((p) => p.stop === undefined && p.to > p.from && meters <= p.to);
  if (!part) return meters <= 0 ? 0 : timelineSeconds;
  const fraction = Math.min(Math.max((meters - part.from) / (part.to - part.from), 0), 1);
  return part.start + (Math.acos(1 - 2 * fraction) / Math.PI) * part.seconds; // inverse of ease
}

// renderPlayhead moves the profile's playhead, its handle, and the scrub
// input to meters along the drawn track.
function renderPlayhead(meters) {
  playhead.setAttribute("x1", meters);
  playhead.setAttribute("x2", meters);
  handle.style.left = `${(meters / Math.max(trackMeters, 1)) * 100}%`;
  scrub.value = String(meters);
}

// renderFlythrough moves the hiker, playhead, panel, and label to the clock.
function renderFlythrough() {
  const index = partIndexAt(clock);
  const part = timeline[index];
  const fraction = part.seconds > 0 ? Math.min((clock - part.start) / part.seconds, 1) : 1;
  hiker = pointAt("meters", part.from + ease(fraction) * (part.to - part.from));
  atPhoto = part.stop !== undefined;
  map.getSource("hiker")?.setData(hikerGeoJSON()); // added on load
  renderPlayhead(hiker.meters);
  // The panel shows the last photo the marker has reached, and the next
  // one downloads on the way.
  const reached = timeline.findLast((p) => p.stop !== undefined && p.start <= clock);
  if (!reached) {
    panel.hidden = true;
  } else if (current !== reached.stop || panel.hidden) {
    select(reached.stop);
    showPanel(stops[reached.stop].photo);
  }
  const next = timeline[index + 1]?.stop; // a move is followed by its photo's pause
  if (!atPhoto && next !== undefined) preload(stops[next].photo);
  renderBar();
}

// enterFlythrough shows the marker and the panel in place of the popup.
function enterFlythrough() {
  if (flying) return;
  flying = true;
  popup.remove();
  showHiker(true);
  renderFlythrough();
}

// leaveFlythrough pauses and hides the marker and the panel.
function leaveFlythrough() {
  pause();
  flying = false;
  showHiker(false);
  panel.hidden = true;
}

function play() {
  if (clock >= timelineSeconds) clock = 0; // ▶ at the end starts again
  enterFlythrough();
  playing = true;
  renderPlayButton();
  cameraBearing = map.getBearing();
  // Fly to the marker first; the frames then keep it in the centre.
  const camera = { center: lonLat(hiker), zoom: Math.max(map.getZoom(), photoZoom), padding: dockPadding() };
  if (map.getTerrain()) camera.bearing = cameraBearing = headingAt(hiker.meters) ?? cameraBearing;
  map.easeTo(camera);
  // Pressing ▶ again before the camera arrives adds another of these, so
  // the loop starts only if none is running.
  map.once("moveend", () => {
    if (!playing || frameRequest !== undefined) return;
    lastFrame = performance.now();
    frameRequest = requestAnimationFrame(frame);
  });
}

function pause() {
  playing = false;
  cancelAnimationFrame(frameRequest);
  frameRequest = undefined;
  renderPlayButton();
}

// renderPlayButton shows ▶ or ❚❚, and stops the label being announced every
// frame while playing.
function renderPlayButton() {
  playButton.classList.toggle("playing", playing);
  playButton.setAttribute("aria-label", playing ? "Pause" : "Play");
  playButton.title = playing ? "Pause (Space)" : "Play (Space)";
  barLabel.setAttribute("aria-live", playing ? "off" : "polite");
}

function frame(now) {
  // A hidden tab gets no frames; don't jump ahead when it shows again.
  const seconds = Math.min((now - lastFrame) / 1000, 0.1);
  lastFrame = now;
  clock = Math.min(clock + seconds, timelineSeconds);
  renderFlythrough();
  followHiker(seconds);
  if (clock >= timelineSeconds) pause();
  else frameRequest = requestAnimationFrame(frame);
}

// followHiker keeps the hiker in the centre of the map; in 3D, it also turns
// the camera, over turnSeconds, to the hiker's heading.
function followHiker(seconds) {
  if (map.isMoving()) return; // zooming by wheel or button; follow again once it ends
  const camera = { center: lonLat(hiker), padding: dockPadding() };
  if (map.getTerrain()) {
    const target = headingAt(hiker.meters) ?? cameraBearing;
    const turn = ((target - cameraBearing + 540) % 360) - 180; // the shorter way round
    cameraBearing += turn * (1 - Math.exp(-seconds / turnSeconds));
    camera.bearing = cameraBearing;
  }
  map.jumpTo(camera);
}

// Photo size: S, M, and L set the width of the photo in the panel and the
// popup.
function renderPhotoSize() {
  document.body.style.setProperty("--photo-width", `${photoWidths[photoSize]}px`);
  for (const button of document.querySelectorAll("[data-size]")) {
    button.setAttribute("aria-pressed", String(button.dataset.size === photoSize));
  }
  fitPopup(); // a bigger photo may no longer fit
  // A wider panel may cover the marker while paused; playing re-centres it each frame.
  if (flying && !playing) map.easeTo({ center: lonLat(hiker), padding: dockPadding() });
}

// Header: the hike's date, distance, duration, and photo count.
function renderStats() {
  const parts = [
    formatTime(points[0].time, hikeOffsetMinutes, { dateStyle: "medium" }), // a GPX track has at least one point
    formatDistance(trackMeters),
    formatDuration(trackSeconds),
    `${data.photos.length} photos`,
  ];
  document.getElementById("stats").textContent = parts.join(" · ");
  for (const button of document.querySelectorAll("[data-units]")) {
    button.setAttribute("aria-pressed", String(button.dataset.units === units));
  }
  scale.setUnit(units);
}

// Start: the controls' listeners, then the first render. Rows in the list
// and pins on the map get theirs when they're made.

previousButton.addEventListener("click", () => step(-1));
nextButton.addEventListener("click", () => step(1));
playButton.addEventListener("click", () => (playing ? pause() : play()));
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") closePhoto();
  if (event.target === scrub) return; // its own arrow keys move it
  if (event.key === "ArrowLeft") step(-1);
  if (event.key === "ArrowRight") step(1);
  // Space on a focused button already clicks it.
  if (event.key === " " && !event.target.closest("button, input, a")) {
    event.preventDefault(); // don't scroll the page
    if (playing) pause();
    else play();
  }
});

// The follow camera's jumpTo cancels any gesture in progress, so pressing on
// the map pauses, leaving the drag or click to MapLibre.
map.getCanvasContainer().addEventListener("pointerdown", pause);

scrub.addEventListener("input", () => {
  pause();
  clock = clockAt(Number(scrub.value));
  enterFlythrough();
  renderFlythrough();
  map.jumpTo({ center: lonLat(hiker), padding: dockPadding() });
});

for (const button of document.querySelectorAll("[data-size]")) {
  button.addEventListener("click", () => {
    photoSize = button.dataset.size;
    saveSetting(photoSizeKey, photoSize);
    renderPhotoSize();
  });
}

for (const button of document.querySelectorAll("[data-units]")) {
  button.addEventListener("click", () => {
    units = button.dataset.units;
    saveSetting(unitsKey, units);
    renderStats();
    renderBar();
    for (const [photo, label] of rowLabels) label.textContent = photoLabel(photo);
  });
}

panel.prepend(closeButton());
renderProfile();
renderPhotoSize();
renderBar();
renderStats();
