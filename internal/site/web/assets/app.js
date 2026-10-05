// retrace's page: the track and photos from the data inlined in index.html,
// on a MapLibre map with a list of photos beside it. It's a classic script,
// not a module, because browsers block module scripts in a page opened from
// a file.
"use strict";

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

// Distance
const earthRadiusMeters = 6371008.8;

// distanceMeters returns the great-circle distance between two points (haversine).
function distanceMeters(a, b) {
  const rad = Math.PI / 180;
  const dLat = (b.lat - a.lat) * rad;
  const dLon = (b.lon - a.lon) * rad;
  const h = Math.sin(dLat / 2) ** 2 + Math.cos(a.lat * rad) * Math.cos(b.lat * rad) * Math.sin(dLon / 2) ** 2;
  return 2 * earthRadiusMeters * Math.asin(Math.sqrt(h));
}

// The drawn track. GPS jitter makes raw points zigzag, and knot where the
// hiker stood still, so each point is averaged over ±smoothingSeconds, then
// thinned to one every minStepMeters. The page data keeps every raw point.
const smoothingSeconds = 5;
const minStepMeters = 2;

// smooth returns line with each point moved to the average position of the
// points within seconds of it.
function smooth(line, seconds) {
  const times = line.map((p) => Date.parse(p.time));
  const windowMillis = seconds * 1000;
  let lo = 0; // first point in the window
  let hi = 0; // first point after the window
  let lat = 0;
  let lon = 0;
  return line.map((_, i) => {
    for (; hi < line.length && times[hi] <= times[i] + windowMillis; hi++) {
      lat += line[hi].lat;
      lon += line[hi].lon;
    }
    for (; times[lo] < times[i] - windowMillis; lo++) {
      lat -= line[lo].lat;
      lon -= line[lo].lon;
    }
    return { lat: lat / (hi - lo), lon: lon / (hi - lo) };
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

const drawnTrack = thin(smooth(points, smoothingSeconds), minStepMeters);
const trackMeters = drawnTrack.reduce((sum, p, i) => (i === 0 ? 0 : sum + distanceMeters(drawnTrack[i - 1], p)), 0);
const trackSeconds = (Date.parse(points.at(-1).time) - Date.parse(points[0].time)) / 1000;

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
// size, then when, with what, and how its pin was placed.
function popupContent(photo) {
  const box = element("div", "popup");
  const link = element("a");
  link.href = photoURL(photo);
  link.target = "_blank";
  const img = element("img");
  img.src = photoURL(photo);
  img.alt = photo.file;
  link.append(img);
  box.append(link, element("p", "strong", photoTime(photo)));

  const camera = [photo.camera?.make, photo.camera?.model].filter(Boolean).join(" ");
  for (const line of [camera, photo.lens, formatSettings(photo.settings)]) {
    if (line) box.append(element("p", "muted", line));
  }
  if (photo.location) box.append(element("p", `note note-${photo.location.source}`, locationNote(photo.location)));
  return box;
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

// Map
const map = new maplibregl.Map({
  container: "map",
  style: "https://tiles.openfreemap.org/styles/liberty",
  bounds: bounds(),
  fitBoundsOptions: { padding: 48 },
  keyboard: false, // the arrow keys step through the photos instead of panning
  maxPitch: 85,
});
map.addControl(new maplibregl.NavigationControl({ visualizePitch: true }));
const scale = new maplibregl.ScaleControl({ unit: units });
map.addControl(scale);

// bounds returns the box around the track and every located photo.
function bounds() {
  const box = new maplibregl.LngLatBounds();
  for (const p of points) box.extend([p.lon, p.lat]);
  for (const photo of data.photos) {
    if (photo.location) box.extend([photo.location.lon, photo.location.lat]);
  }
  return box;
}

map.on("load", () => {
  map.addSource("track", {
    type: "geojson",
    data: { type: "Feature", geometry: { type: "LineString", coordinates: drawnTrack.map((p) => [p.lon, p.lat]) } },
  });
  map.addLayer({
    id: "track",
    type: "line",
    source: "track",
    layout: { "line-join": "round", "line-cap": "round" },
    paint: { "line-color": "#d9480f", "line-width": 3 },
  });
  addTerrain();
});

// Terrain: a 3D button draws the map over Mapterhorn's elevation tiles, with
// hillshading. The page starts in 2D, which downloads no terrain tiles,
// unless the viewer chose 3D before.
const terrainKey = "retrace.terrain";
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
  // Pins are raised onto the terrain only when the camera moves, so place
  // them again when terrain tiles arrive.
  map.on("sourcedata", (event) => {
    if (event.sourceId !== "terrain" || !event.isSourceLoaded) return;
    for (const { marker } of stops) marker.setLngLat(marker.getLngLat());
  });
  if (loadSetting(terrainKey, ["2d", "3d"], "2d") === "3d") map.setTerrain(terrain);
}

// Photos: a pin on the map and a row in the list for each located photo, and
// a row under "Not on the map" for the rest. The located photos, in time
// order, are the carousel's stops; the arrows step through them.
const stops = []; // {photo, marker, row} for each located photo
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
  const pin = element("div", `pin pin-${photo.location.source}`);
  pin.title = photoTime(photo);
  // The content is built when the popup first opens: an <img> starts
  // downloading as soon as it has a src, even before it's on the page.
  // Always above the pin: MapLibre would put a tall photo below it.
  const popup = new maplibregl.Popup({ offset: 12, maxWidth: "none", anchor: "bottom" });
  popup.once("open", () => {
    const content = popupContent(photo);
    content.querySelector("img").addEventListener("load", () => fitPopup(popup));
    popup.setDOMContent(content);
  });
  popup.on("open", () => {
    select(index); // whether opened by the pin, the list, or an arrow
    fitPopup(popup);
  });
  popup.on("close", () => row.classList.remove("current"));
  const marker = new maplibregl.Marker({ element: pin })
    .setLngLat([photo.location.lon, photo.location.lat])
    .setPopup(popup)
    .addTo(map);
  stops.push({ photo, marker, row });

  row.querySelector("button").addEventListener("click", () => show(index));
  row.addEventListener("mouseenter", () => pin.classList.add("active"));
  row.addEventListener("mouseleave", () => pin.classList.remove("active"));
  document.getElementById("photos").append(row);
}

// show flies to the photo at index in stops and opens its popup.
function show(index) {
  const { photo, marker } = stops[index];
  select(index);
  const camera = { center: marker.getLngLat(), zoom: Math.max(map.getZoom(), 14) };
  if (map.getTerrain()) {
    // Look the way the hiker was walking, so the camera is over ground they
    // had crossed rather than behind the next ridge.
    camera.bearing = travelBearing(photo) ?? map.getBearing();
  }
  map.flyTo(camera); // fitPopup makes room for the popup once it opens

  // MapLibre won't open the popup of a pin it counts as hidden behind
  // terrain, which it may until the map has redrawn at the destination.
  const openPopup = () => {
    if (current === index && !marker.getPopup().isOpen()) marker.togglePopup();
  };
  map.once("moveend", () => {
    if (!map.getTerrain()) {
      openPopup();
      return;
    }
    // flyTo guesses the ground height at the destination before its terrain
    // has loaded, so it can land off the pin; centre on it again.
    if (current === index) map.jumpTo({ center: marker.getLngLat() });
    map.once("idle", openPopup);
  });
}

// fitPopup moves the map down if popup reaches above its top edge, so all of
// a tall photo shows. It waits a frame: MapLibre fires "open" before the
// popup is on the page.
function fitPopup(popup) {
  requestAnimationFrame(() => {
    const element = popup.getElement();
    if (!popup.isOpen() || !element) return;
    const margin = 8;
    const overflow = map.getContainer().getBoundingClientRect().top + margin - element.getBoundingClientRect().top;
    if (overflow > 0) map.panBy([0, -overflow]);
  });
}

// travelBearing returns the direction the hiker was walking when photo was
// taken, from the last track point at least 100 m before it, in degrees
// clockwise from north; undefined without a time or such a point.
function travelBearing(photo) {
  if (!photo.time) return undefined;
  const to = photo.location;
  const taken = Date.parse(photo.time);
  let i = points.findLastIndex((p) => Date.parse(p.time) <= taken);
  while (i >= 0 && distanceMeters(points[i], to) < 100) i--;
  if (i < 0) return undefined;
  const from = points[i];
  const rad = Math.PI / 180;
  const y = Math.sin((to.lon - from.lon) * rad) * Math.cos(to.lat * rad);
  const x = Math.cos(from.lat * rad) * Math.sin(to.lat * rad) - Math.sin(from.lat * rad) * Math.cos(to.lat * rad) * Math.cos((to.lon - from.lon) * rad);
  return Math.atan2(y, x) / rad;
}

// select makes the photo at index the current one: its popup alone is open,
// and its row is highlighted.
function select(index) {
  current = index;
  stops.forEach((stop, i) => {
    stop.row.classList.toggle("current", i === index);
    if (i !== index && stop.marker.getPopup().isOpen()) stop.marker.togglePopup();
  });
  stops[index].row.scrollIntoView({ block: "nearest" });
  renderCarousel();
}

// Carousel: the bar at the bottom of the map.
const previousButton = document.getElementById("previous");
const nextButton = document.getElementById("next");

function renderCarousel() {
  const position = document.getElementById("position");
  if (current < 0) {
    position.textContent = `${stops.length} photos on the map`;
  } else {
    position.textContent = `${current + 1} / ${stops.length} · ${photoLabel(stops[current].photo)}`;
  }
  // The ends don't wrap: a hike has a start and an end.
  previousButton.disabled = current <= 0;
  nextButton.disabled = current >= stops.length - 1;
}

// step shows the photo by stops after the current one; before any, → shows
// the first. It stops at the ends.
function step(by) {
  const index = current < 0 ? (by > 0 ? 0 : -1) : current + by;
  if (index >= 0 && index < stops.length) show(index);
}

previousButton.addEventListener("click", () => step(-1));
nextButton.addEventListener("click", () => step(1));
document.addEventListener("keydown", (event) => {
  if (event.key === "ArrowLeft") step(-1);
  if (event.key === "ArrowRight") step(1);
});

// Photo size: the width of the photo in a popup.
const photoSizeKey = "retrace.photoSize";
const photoWidths = { s: 320, m: 480, l: 720 };

let photoSize = loadSetting(photoSizeKey, Object.keys(photoWidths), "m");

function renderPhotoSize() {
  document.body.style.setProperty("--photo-width", `${photoWidths[photoSize]}px`);
  for (const button of document.querySelectorAll("[data-size]")) {
    button.setAttribute("aria-pressed", String(button.dataset.size === photoSize));
  }
  if (current >= 0) fitPopup(stops[current].marker.getPopup()); // a bigger photo may no longer fit
}

for (const button of document.querySelectorAll("[data-size]")) {
  button.addEventListener("click", () => {
    photoSize = button.dataset.size;
    saveSetting(photoSizeKey, photoSize);
    renderPhotoSize();
  });
}

renderPhotoSize();
renderCarousel();

// Header
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

for (const button of document.querySelectorAll("[data-units]")) {
  button.addEventListener("click", () => {
    units = button.dataset.units;
    saveSetting(unitsKey, units);
    renderStats();
    renderCarousel();
    for (const [photo, label] of rowLabels) label.textContent = photoLabel(photo);
  });
}

renderStats();
