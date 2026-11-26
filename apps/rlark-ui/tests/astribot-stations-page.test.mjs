import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const page = await readFile(new URL("../src/astribot/pages/Stations.tsx", import.meta.url), "utf8");
const card = await readFile(new URL("../src/astribot/pages/StationCard.tsx", import.meta.url), "utf8");
const panel = await readFile(new URL("../src/astribot/pages/StationDetailPanel.tsx", import.meta.url), "utf8");
const dialog = await readFile(new URL("../src/astribot/pages/CameraPreviewDialog.tsx", import.meta.url), "utf8");
const fields = await readFile(new URL("../src/astribot/pages/StationFields.tsx", import.meta.url), "utf8");
const cardsStyles = await readFile(new URL("../src/astribot/pages/StationsCards.css", import.meta.url), "utf8");
const power = await readFile(new URL("../src/astribot/components/useRobotPower.ts", import.meta.url), "utf8");
const controls = await readFile(new URL("../src/astribot/components/RobotControls.tsx", import.meta.url), "utf8");

test("the site-node page keeps the compact toolbar and groups robot clusters apart", () => {
  assert.match(page, /className="page-content resource-page stations-page"/);
  assert.match(page, /className="stations-phase-tabs"/);
  assert.match(page, /className="stations-toolbar"/);
  assert.match(page, /className="stations-group stations-robot-group"/);
  assert.match(page, /className="stations-cluster"/);
  assert.match(page, /<CardGrid/);
  assert.match(page, /<StationDetailPanel/);
  assert.match(page, /<CameraPreviewDialog/);
  assert.match(page, /<AdmissionRecords/);
  // The hero, the four summary cards and the station drawer are gone.
  assert.doesNotMatch(page, /stations-hero|stations-overview-grid|StationDrawer|stations-view-detail/);
  assert.doesNotMatch(page, /selectedID|drawerTrigger|drawerDeleteRequest/);
});

test("one station opens at a time and the detail sits under its card row", () => {
  assert.match(page, /const \[openKey, setOpenKey\] = useState\(""\)/);
  assert.match(page, /expanded=\{openKey === key\}/);
  assert.match(page, /openKey === keyOf\(item\)/);
  assert.match(page, /const toggleOpen =/);
  assert.match(page, /const closePanel =/);
  assert.match(page, /focusExpander/);
  assert.match(page, /requestAnimationFrame/);
  // The old default-open pass and its multi-open Set are gone.
  assert.doesNotMatch(page, /defaultsApplied|toggledKeys|openKeys/);
  // The detail is a row-spanning child of the card grid, grouped by the measured column count.
  assert.match(page, /useGridColumns/);
  assert.match(page, /getComputedStyle\(element\)\.gridTemplateColumns/);
  assert.match(page, /opened && renderPanel\(opened\)/);
});

test("a robot card carries the camera wall and the control plane is a compact row", () => {
  assert.match(card, /function CameraWall/);
  assert.match(card, /IntersectionObserver/);
  assert.match(card, /!visible \|\| expanded/);
  assert.match(card, /onZoom\(cell\.view, event\.currentTarget\)/);
  assert.match(card, /aria-haspopup="dialog"/);
  assert.match(card, /station-camera-wall/);
  assert.match(card, /station-camera-state/);
  assert.match(card, /variant === "control"/);
  assert.match(card, /station-control-row/);
  assert.match(card, /aria-expanded=\{expanded\}/);
  assert.match(card, /aria-controls=\{detailsID\}/);
  // Details are no longer rendered inside the card.
  assert.doesNotMatch(card, /stations-card-details|stations-card-tabs|role="tablist"/);
  assert.doesNotMatch(card, /<NodeCameras/);
});

test("power and driver actions are on the card and report progress and a result", () => {
  assert.match(card, /driverAction/);
  assert.match(card, /启动驱动|关闭驱动/);
  assert.match(card, /power\.press\(driverAction, driverLabel\)/);
  assert.match(page, /stations-power-toast/);
  assert.match(page, /power\.activeLabel/);
  assert.match(page, /power\.failed/);
  assert.match(page, /power\.clear/);
  assert.match(page, /showMessage=\{false\}/);
  assert.match(power, /const \[activeLabel/);
  assert.match(power, /const \[failed/);
  assert.match(power, /const clear =/);
  assert.match(power, /failures\.length > 0/);
  assert.match(controls, /showMessage/);
  assert.match(cardsStyles, /stations-power-toast/);
  assert.match(cardsStyles, /stations-power-spinner/);
});

test("a faulted power state and the Node's power note are visible", () => {
  assert.match(card, /state === "fault"/);
  assert.match(card, /硬件报错/);
  assert.match(controls, /回位不可用/);
  assert.match(controls, /电源状态为硬件报错/);
  assert.match(panel, /powerNote/);
  assert.match(page, /rlark\.io\/power/);
});

test("the home-pose control explains that it is hold-to-move and why it is unavailable", () => {
  assert.match(controls, /按住回位|Hold home/);
  assert.match(controls, /robot-pose-hint/);
  assert.match(controls, /按住不放才会移动|Hold the button to move/);
  assert.match(controls, /已停止|Stopped/);
  assert.match(controls, /驱动没在跑，先启动驱动|Start the driver first/);
});

test("the detail panel uses the full row for identity, network and conditions", () => {
  assert.match(panel, /station-detail-panel/);
  assert.match(panel, /station-detail-columns/);
  assert.match(panel, /station-detail-conflict/);
  assert.match(panel, /conflictUID/);
  assert.match(panel, /conditionView/);
  assert.match(panel, /station-condition-list/);
  assert.match(panel, /station-condition-pill/);
  assert.match(panel, /<NodeLinks/);
  assert.match(panel, /<NodeCameras/);
  assert.match(panel, /CopyValue/);
  assert.match(panel, /stationDeleteReason/);
  assert.match(panel, /aria-describedby/);
  assert.match(panel, /onKeyDown/);
  assert.match(panel, /event\.key !== "Escape"/);
  assert.match(panel, /titleRef\.current\?\.focus\(\{ preventScroll: true \}\)/);
  // Opening a detail must not scroll the page away from the card row the user clicked.
  assert.doesNotMatch(panel, /scrollIntoView/);
  for (const label of ["重发入网码", "作废入网码", "删除记录"]) {
    assert.ok(panel.includes(label), `the detail offers ${label}`);
  }
  assert.match(fields, /export function NetworkBadge/);
  assert.match(fields, /export function CopyValue/);
});

test("the camera dialog polls only while open and returns focus to its button", () => {
  assert.match(dialog, /showModal\(\)/);
  assert.match(dialog, /addEventListener\("close"/);
  assert.match(dialog, /cameraFramePath/);
  assert.match(dialog, /setInterval/);
  assert.match(dialog, /onDismiss/);
  assert.match(dialog, /aria-labelledby="station-camera-dialog-title"/);
  assert.match(page, /const closeZoom =/);
  assert.match(page, /trigger\?\.focus\(\)/);
});

test("the card grid caps width, keeps stock cards compact and aligns row bottoms", () => {
  assert.match(cardsStyles, /\/\* The grid picks its tracks from the page width/);
  assert.match(cardsStyles, /\.stations-page \.stations-card-grid/);
  assert.match(cardsStyles, /repeat\(auto-fill, minmax\(340px, var\(--station-track\)\)\)/);
  assert.match(cardsStyles, /\.stations-card-grid\.is-stock-grid/);
  assert.match(cardsStyles, /minmax\(300px, var\(--station-track\)\)/);
  assert.match(cardsStyles, /--station-track: clamp\(300px/);
  assert.match(cardsStyles, /max-width: 460px/);
  assert.match(cardsStyles, /max-width: 360px/);
  assert.match(cardsStyles, /align-items: stretch/);
  assert.match(cardsStyles, /margin-top: auto/);
  assert.match(cardsStyles, /station-camera-wall/);
  assert.match(cardsStyles, /grid-column: 1 \/ -1/);
  assert.match(cardsStyles, /container-type: inline-size/);
  assert.match(cardsStyles, /@container \(width < 960px\)/);
  assert.match(cardsStyles, /@container \(width < 640px\)/);
  assert.match(cardsStyles, /station-detail-columns/);
  // The hidden-details shell from the previous in-card design is not part of the new card.
  assert.doesNotMatch(cardsStyles, /\.stations-card-details/);
});
