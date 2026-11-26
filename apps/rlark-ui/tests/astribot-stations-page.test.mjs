import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const page = await readFile(new URL("../src/astribot/pages/Stations.tsx", import.meta.url), "utf8");
const card = await readFile(new URL("../src/astribot/pages/StationCard.tsx", import.meta.url), "utf8");
const panel = await readFile(new URL("../src/astribot/pages/StationDetailPanel.tsx", import.meta.url), "utf8");
const dialog = await readFile(new URL("../src/astribot/pages/CameraPreviewDialog.tsx", import.meta.url), "utf8");
const dialogShell = await readFile(new URL("../src/astribot/pages/StationDetailDialog.tsx", import.meta.url), "utf8");
const fields = await readFile(new URL("../src/astribot/pages/StationFields.tsx", import.meta.url), "utf8");
const nodeLinks = await readFile(new URL("../src/astribot/components/NodeLinks.tsx", import.meta.url), "utf8");
const cardsStyles = await readFile(new URL("../src/astribot/pages/StationsCards.css", import.meta.url), "utf8");
const power = await readFile(new URL("../src/astribot/components/useRobotPower.ts", import.meta.url), "utf8");
const controls = await readFile(new URL("../src/astribot/components/RobotControls.tsx", import.meta.url), "utf8");

test("the site-node page keeps the toolbar and groups robot clusters apart in a bounded column", () => {
  assert.match(page, /className="page-content resource-page stations-page"/);
  assert.match(page, /className="stations-frame"/);
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
  // The wall is full-bleed: the shell's page gutters collapse and the column has no cap.
  assert.match(cardsStyles, /\.stations-page\.page-content { padding: 18px 12px 26px; }/);
  assert.match(cardsStyles, /\.stations-page \.stations-frame/);
  assert.match(cardsStyles, /max-width: none/);
  // Column counts follow the frame's real width, not the window's.
  assert.match(cardsStyles, /container-name: stations-frame/);
  assert.match(cardsStyles, /@container stations-frame \(max-width: 1160px\)/);
});

test("one station opens at a time and its detail opens as a dialog", () => {
  assert.match(page, /const \[openKey, setOpenKey\] = useState\(""\)/);
  assert.match(page, /expanded=\{openKey === key\}/);
  assert.match(page, /aria-expanded=\{openKey === key\}/);
  assert.match(page, /const toggleOpen =/);
  assert.match(page, /const closePanel =/);
  assert.match(page, /focusExpander/);
  assert.match(page, /requestAnimationFrame/);
  // The old default-open pass and its multi-open Set are gone.
  assert.doesNotMatch(page, /defaultsApplied|toggledKeys|openKeys/);
  // The detail renders once, in a native dialog from the page, not inside the grid rows.
  assert.match(page, /<StationDetailDialog/);
  assert.match(page, /const detailItem = openKey \? items\.find/);
  assert.doesNotMatch(page, /useGridColumns|opened && renderPanel/);
  assert.match(card, /aria-haspopup="dialog"/);
  assert.match(page, /aria-haspopup="dialog"/);
});

test("a robot renders as a camera-wall tile and the control plane is a compact row", () => {
  assert.match(card, /function CameraWall/);
  assert.match(card, /IntersectionObserver/);
  assert.match(card, /!visible \|\| expanded/);
  assert.match(card, /station-tile-media/);
  assert.match(card, /station-tile-main/);
  assert.match(card, /station-tile-overlay is-top/);
  assert.match(card, /station-tile-overlay is-bottom/);
  assert.match(card, /station-tile-strip/);
  assert.match(card, /station-camera-state/);
  // Camera frames stay clickable and enlarge; they never expand the tile.
  assert.match(card, /onZoom\(big\.view, event\.currentTarget\)/);
  assert.match(card, /onZoom\(cell\.view, event\.currentTarget\)/);
  assert.match(card, /aria-haspopup="dialog"/);
  // The tile overlays carry identity, state and the expander.
  assert.match(card, /station-tile-expand/);
  assert.match(card, /aria-expanded=\{expanded\}/);
  assert.match(card, /aria-controls=\{detailsID\}/);
  assert.match(card, /station-tile-phase/);
  assert.match(card, /station-tile-power/);
  assert.match(card, /station-tile-action/);
  assert.match(card, /variant === "control"/);
  assert.match(card, /station-control-row/);
  // An alert robot (power fault, e-stop, failed self-check, maintenance) rings its tile.
  assert.match(card, /tileAlert/);
  assert.match(card, /station-tile\$\{expanded \? " is-expanded" : ""\}/);
  // Details are no longer rendered inside the card.
  assert.doesNotMatch(card, /stations-card-details|stations-card-tabs|role="tablist"/);
  assert.doesNotMatch(card, /<NodeCameras/);
});

test("power and driver actions are on the tile and report progress and a result", () => {
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
  // Inside a tile the hints float below the overlay instead of pushing the picture.
  assert.match(cardsStyles, /station-tile-actions \.robot-pose-hint/);
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
  // The title is focused by the dialog after showModal; the panel must not try earlier, when nothing inside
  // the not-yet-open dialog can hold focus (that left focus on the section itself).
  assert.match(panel, /data-detail-title tabIndex=\{-1\}/);
  assert.doesNotMatch(panel, /titleRef|useEffect\(\(\) => \{\s*\/\/ Keep the keyboard focus/);
  assert.match(dialogShell, /showModal\(\)/);
  assert.match(page, /detail\.closedBy = zoom \? "none" : "closerequest"/);
  assert.match(dialogShell, /querySelector<HTMLElement>\("\[data-detail-title\]"\)\?\.focus\(\{ preventScroll: true \}\)/);
  // The identity-verification note is a muted line unless it is a mismatch, which stays an alert like it is on
  // the tile and the inventory row.
  assert.match(panel, /<small className=\{siteNode\.identity_verified === "False" \? "is-alert" : undefined\}>/);
  assert.match(cardsStyles, /station-detail-field dd strong small\.is-alert/);
  // Opening a detail must not scroll the page away from the tile the user clicked.
  assert.doesNotMatch(panel, /scrollIntoView/);
  assert.doesNotMatch(dialogShell, /scrollIntoView/);
  for (const label of ["重发入网码", "作废入网码", "删除记录"]) {
    assert.ok(panel.includes(label), `the detail offers ${label}`);
  }
  assert.match(fields, /export function NetworkBadge/);
  assert.match(fields, /export function CopyValue/);
});

test("station details use equal-width columns and stack on narrow dialogs", () => {
  assert.match(cardsStyles, /\.station-detail-columns\s*\{\s*grid-template-columns: repeat\(3, minmax\(0, 1fr\)\)/);
  assert.match(cardsStyles, /@container \(width < 960px\)\s*\{\s*\.stations-page \.station-detail-columns\s*\{ grid-template-columns: repeat\(2, minmax\(0, 1fr\)\)/);
  assert.match(cardsStyles, /@container \(width < 640px\)\s*\{\s*\.stations-page \.station-detail-columns\s*\{ grid-template-columns: minmax\(0, 1fr\)/);
});

test("Hub links show a collapsed summary below the station detail columns", () => {
  assert.match(panel, /<\/div>\s*\{siteNode && <NodeLinks[^>]+collapsible \/>\}/);
  assert.match(nodeLinks, /<details className="station-network-links">\s*<summary>[\s\S]+nodeLinkLine\(site, zh\)/);
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

test("the robot grid fills its row and inventory nodes render as quiet rows", () => {
  // Three tracks that fill the row, stepping down on narrower screens.
  assert.match(cardsStyles, /\.stations-page \.stations-card-grid\.is-robot-grid/);
  assert.match(cardsStyles, /repeat\(3, minmax\(0, 1fr\)\)/);
  assert.match(cardsStyles, /repeat\(2, minmax\(0, 1fr\)\)/);
  // The wall has no panel chrome around it.
  assert.match(cardsStyles, /\.stations-group\.stations-robot-group/);
  assert.match(cardsStyles, /background: transparent/);
  // Inventory rows: icon, name, dot status, mono facts, chevron; hover tone; hairline separators.
  assert.match(page, /className="stations-inv-list"/);
  assert.match(page, /station-inv-row/);
  assert.match(page, /station-inv-status/);
  assert.match(page, /station-inv-facts/);
  assert.match(page, /station-inv-expand/);
  assert.match(cardsStyles, /\.station-inv-row/);
  assert.match(cardsStyles, /\.station-inv-expand/);
  // The robot cluster head reports driver readiness and camera trouble at a glance.
  assert.match(page, /驱动就绪/);
  assert.match(page, /相机异常/);
});

test("the tile keeps its reserved hint slot and mono facts row", () => {
  assert.match(card, /station-tile-facts/);
  assert.match(cardsStyles, /\.station-tile-facts/);
  assert.match(cardsStyles, /font-variant-numeric: tabular-nums/);
});
