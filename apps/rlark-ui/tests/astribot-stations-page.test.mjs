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
const restartHook = await readFile(new URL("../src/astribot/components/useNodeRestart.ts", import.meta.url), "utf8");
const restartLib = await readFile(new URL("../src/astribot/lib/nodeRestart.ts", import.meta.url), "utf8");
const stationDisplay = await readFile(new URL("../src/astribot/lib/stationDisplay.ts", import.meta.url), "utf8");
const stationsApi = await readFile(new URL("../src/astribot/api.ts", import.meta.url), "utf8");

test("the site-node page keeps the toolbar and groups robot clusters apart in a bounded column", () => {
  assert.match(page, /className="page-content resource-page stations-page"/);
  assert.match(page, /className="stations-frame"/);
  assert.match(page, /className="stations-phase-tabs"/);
  assert.match(page, /className="stations-toolbar"/);
  assert.match(page, /className="stations-group stations-robot-group"/);
  assert.match(page, /className="stations-table-clusters"/);
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
  assert.match(card, /visible && foreground && !expanded/);
  assert.match(card, /station-tile-media/);
  assert.match(card, /station-tile-main/);
  assert.match(card, /station-tile-overlay is-top/);
  // The picture keeps its whole lower edge: only the identity overlay is left, and state plus controls sit in a
  // dock that follows the media (main camera and strip) instead of covering it.
  assert.doesNotMatch(card, /station-tile-overlay is-bottom/);
  assert.doesNotMatch(card, /overlayBottom/);
  assert.match(card, /<footer className="station-tile-dock">/);
  assert.match(card, /station-tile-dock-status/);
  assert.match(card, /overlayTop=\{overlayTop\} \/>\s*\{dock\}/);
  assert.match(card, /station-tile-strip/);
  assert.match(card, /station-camera-state/);
  // Camera frames stay clickable and enlarge; they never expand the tile.
  assert.match(card, /onZoom\(big\.view, event\.currentTarget, fps\)/);
  assert.match(card, /onZoom\(cell\.view, event\.currentTarget, fps\)/);
  assert.match(card, /aria-haspopup="dialog"/);
  // The identity overlay carries the details entry, labelled in words rather than an icon alone; the id, the
  // expanded state and the control target are unchanged, so focus can still return to it.
  assert.match(card, /station-tile-expand/);
  assert.match(card, /station-tile-expand-label/);
  assert.match(card, /zh \? "详情" : "Details"/);
  assert.match(card, /id=\{expanderID\}/);
  assert.match(card, /aria-expanded=\{expanded\}/);
  assert.match(card, /aria-controls=\{detailsID\}/);
  assert.match(card, /station-tile-phase/);
  assert.match(card, /station-tile-pill is-desired/);
  assert.match(card, /station-tile-pill is-actual/);
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

test("power and driver actions are always on the tile, grouped by function", () => {
  // Both halves of each pair are fixed on the card; the current state only disables what it cannot act on.
  assert.match(card, /actionGroups/);
  assert.match(card, /action: "power_on"/);
  assert.match(card, /action: "power_off"/);
  assert.match(card, /action: "driver_on"/);
  assert.match(card, /action: "driver_off"/);
  assert.match(card, /label: zh \? "电源" : "Power"/);
  assert.match(card, /label: zh \? "驱动" : "Driver"/);
  assert.match(card, /station-tile-action-group/);
  assert.match(card, /station-tile-action-group-label/);
  assert.match(card, /RobotPoseButton/);
  // A disabled action explains the state that blocks it; the note line says it visibly.
  assert.match(card, /robotPowerActionBlocked\(action, powerNow, zh\)/);
  assert.match(card, /power\.press\(action, label\)/);
  assert.match(card, /robotPowerStateNote\(powerNow, zh\)/);
  assert.match(card, /station-tile-action-note/);
  assert.match(cardsStyles, /station-tile-action-groups/);
  assert.match(cardsStyles, /station-tile-action-group/);
  assert.match(cardsStyles, /station-tile-action-note/);
  // The dock is a surface of the page palette under the picture, never the picture's dark glass.
  assert.match(cardsStyles, /\.stations-page \.station-tile-dock \{/);
  assert.match(cardsStyles, /\.stations-page \.station-tile-dock-status \{/);
  assert.match(cardsStyles, /\.stations-page \.station-tile-pill \{/);
  assert.doesNotMatch(cardsStyles, /station-tile-overlay\.is-bottom/);
  // A single-column tile sets the state beside the controls instead of stacking them.
  assert.match(cardsStyles, /\.stations-page \.station-tile-dock \{ flex-direction: row; flex-wrap: wrap; align-items: center; justify-content: space-between;/);
})

test("a power request reports sending, acceptance and the robot's own observation on its card", () => {
  assert.match(card, /station-tile-request/);
  assert.match(card, /powerRequestText/);
  assert.match(card, /已受理/);
  assert.match(card, /已生效/);
  assert.match(card, /未观测到/);
  assert.match(card, /robotPowerActionTiming/);
  assert.match(cardsStyles, /station-tile-request/);
  // Per-robot request state in the hook, followed on the Node source the page already uses, not a refresh loop.
  assert.match(power, /export interface RobotPowerRequest/);
  assert.match(power, /const \[requests, setRequests\]/);
  assert.match(power, /OBSERVE_INTERVAL_MS = 5000/);
  assert.match(power, /ACTION_WINDOW_MS/);
  assert.match(power, /watchRef\.current\?\.observe/);
  assert.match(power, /phase: "unconfirmed"/);
  assert.match(power, /phase: "observed"/);
  assert.match(page, /nodesApi\.get\(target\.node, \{ namespace: target\.namespace \}\)/);
  assert.match(page, /observe: observeRobotPower/);
  assert.match(page, /rlark\.io\/power/);
  // The page-level toast still reports the Gateway's answer.
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

test("the home-pose control explains hold-to-move, the first hold, and why it is unavailable", () => {
  assert.match(controls, /按住回位|Hold home/);
  assert.match(controls, /robot-pose-hint/);
  assert.match(controls, /按住不放才会移动|Hold the button to move/);
  assert.match(controls, /已停止|Stopped/);
  assert.match(controls, /驱动没在跑，先启动驱动|Start the driver first/);
  // Before the first hold is acknowledged the button says it is requesting, not moving.
  assert.match(controls, /"confirming"/);
  assert.match(controls, /setPhase\("confirming"\)/);
  assert.match(controls, /请求中…|Requesting…/);
  assert.match(controls, /hold\.confirmed/);
  assert.match(controls, /is-confirming/);
  // A late hold receipt cannot restart a released hold: the session identity is checked before any phase change.
  assert.match(controls, /if \(held\.current !== hold\) return;/);
  assert.match(controls, /session\.state = "releasing"/);
  assert.match(controls, /停止中…|Stopping…/);
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

test("the camera dialog streams at the selected rate and returns focus to its button", () => {
  assert.match(dialog, /showModal\(\)/);
  assert.match(dialog, /addEventListener\("close"/);
  assert.match(dialog, /cameraStreamPath\(node, namespace, view, fps\)/);
  assert.match(dialog, /FPS_OPTIONS = CAMERA_FPS_OPTIONS/);
  assert.match(dialog, /sameCameraFrame\(lastFrame, jpeg\)/);
  assert.match(dialog, /active\?\.abort\(\)/);
  assert.match(card, /setTimeout\(\(\) => void ask\(\), cameraMediaScheduler\.nextSnapshotDelayMs\(\)\)/);
  assert.match(card, /readCameraAge\(response\.headers\.get\("X-Age-Seconds"\)\)/);
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

test("the card top line carries the main camera name, frame age and fetch time", () => {
  // The main camera's name, frame age and fetch time sit with the robot's name, above the model / UID line.
  assert.match(card, /className="station-tile-title"[\s\S]*?<small ref=\{cameraMetrics\} className="station-tile-camera-metrics"/);
  assert.match(card, /station-tile-title[\s\S]*?UID \{shortUID\(station\.spec\.nodeUID\)\}/);
  assert.match(card, /`\$\{big\.name\} · \$\{zh \? "帧龄" : "Frame age"\} \$\{formatCameraMilliseconds\(age\)\}`/);
  assert.match(card, /`\$\{zh \? "取图" : "Fetch"\} \$\{formatCameraMilliseconds\(shot\.fetchMs\)\}`/);
  assert.match(cardsStyles, /\.stations-page \.station-tile-top \{ display: flex; flex-wrap: wrap/);
  assert.match(cardsStyles, /@container stations-frame \(max-width: 720px\)/);
});

test("the card preview rate sits left of the online label and defaults to the lowest tier", () => {
  assert.match(card, /CAMERA_FPS_OPTIONS\.map/);
  assert.match(card, /<select value=\{previewFps\}/);
  assert.match(card, /className="station-tile-fps"[\s\S]*?className=\{`station-tile-phase status/);
  assert.match(card, /useState<CameraFps>\(DEFAULT_CAMERA_FPS\)/);
  assert.match(dialog, /useState<CameraFps>\(initialFps \?\? 10\)/);
  assert.match(page, /initialFps=\{zoom\.fps\}/);
  assert.match(page, /onZoom: \(camera, trigger, fps\) => cardHandlers\.current\.openZoom\(item, camera, trigger, fps\)/);
  assert.match(cardsStyles, /\.stations-page \.station-tile-fps select/);
});

test("changing the rate restarts the card's single stream at the new fps", () => {
  // The main camera streams through the existing camera stream path, so the rate is a real request parameter.
  assert.match(card, /cameraStreamPath\(node, namespace, streamView, fps\)/);
  assert.match(card, /sameCameraFrame\(lastFrame, jpeg\)/);
  assert.match(card, /URL\.createObjectURL\(new Blob\(\[jpeg\], \{ type: "image\/jpeg" \}\)\)/);
  // Fetch time is measured to the first complete JPEG, not merely to the response headers.
  assert.match(card, /firstFrameFetch \?\?= receivedAt - started/);
  // Only one subscription exists: the render for a new rate aborts the old one first.
  assert.match(card, /\[streaming, node, namespace, streamView, fps, zh\]/);
  assert.match(card, /const active = connection;\s*connection = undefined;\s*active\?\.abort\(\)/);
});

test("high-frequency camera numbers never re-render React", () => {
  // Frame age and fetch time are written to the DOM node on an animation-frame throttle.
  assert.match(card, /target\.textContent = \[/);
  assert.match(card, /window\.requestAnimationFrame\(\(\) => \{ painting = 0; paint\(\); \}\)/);
  // One timer paints the frame-age line and closes the measured-rate window; no React state changes.
  assert.match(card, /window\.setInterval\(\(\) => \{\s*if \(document\.hidden\) return;\s*paint\(\);\s*if \(!measuredAt\) return;/);
  assert.doesNotMatch(card, /setNow/);
  // A frame kept longer than ten seconds reads stale even if the camera reported a smaller age.
  assert.match(card, /at - shot\.receivedAt >= 10000 \|\| \(age !== undefined && age >= 5000\)/);
  assert.match(card, /Keep the last frame's record so the metrics line can turn stale/);
});

test("the card's rate selector shows the rate the browser actually received", () => {
  // Distinct frames are counted after the identical-frame check, in windows of about one second.
  assert.match(card, /sameCameraFrame\(lastFrame, jpeg\)\) continue;\s*lastFrame = jpeg;\s*distinct\+\+;/);
  assert.match(card, /measuredAt = performance\.now\(\);\s*for await \(const \{ jpeg, capturedAtMs, ageSeconds \} of readCameraFrames\(response\)\)/);
  assert.match(card, /showMeasuredFps\(distinct \* 1000 \/ \(now - measuredAt\)\)/);
  // The reading sits beside the selector inside the same label, so a wrapping top line never splits them.
  assert.match(card, /className="station-tile-fps"[\s\S]*?<select value=\{previewFps\}[\s\S]*?<small ref=\{measuredFps\} className="station-tile-measured-fps"/);
  assert.match(cardsStyles, /\.stations-page \.station-tile-fps \.station-tile-measured-fps/);
  // Until measured, while the stream is down, in a hidden tab and in still-frame fallback it reads —.
  assert.match(card, /rate === null \? "—" : `\$\{zh \? "实测" : "Measured"\} \$\{rate\.toFixed\(1\)\} FPS`/);
  assert.match(card, /const clear = \(\) => \{[\s\S]*?showMeasuredFps\(null\)/);
  assert.match(card, /const disconnect = \(\) => \{[\s\S]*?showMeasuredFps\(null\)/);
  assert.match(card, /connection = undefined;\s*distinct = 0;\s*measuredAt = 0;\s*showMeasuredFps\(null\)/);
  // The dialog keeps its own measured rate, unchanged by the card's.
  assert.match(dialog, /station-preview-actual-fps/);
});

test("the card stream pauses in a hidden tab, falls back to a still and cleans up completely", () => {
  assert.match(card, /if \(document\.hidden \|\| connection\) return;/);
  assert.match(card, /document\.addEventListener\("visibilitychange", visibility\)/);
  assert.match(card, /document\.removeEventListener\("visibilitychange", visibility\)/);
  assert.match(card, /disconnect\(\);[\s\S]*?live\.current = undefined;[\s\S]*?paint\(\);/);
  // A gateway without a stream still shows its still-frame endpoint on the card.
  assert.match(card, /cameraFramePath\(node, namespace, streamView\)/);
  // Unmount and rate changes release timers, subscriptions, animation frames and object URLs.
  assert.match(card, /window\.clearInterval\(tick\)/);
  assert.match(card, /window\.cancelAnimationFrame\(painting\)/);
  assert.match(card, /if \(url\) URL\.revokeObjectURL\(url\)/);
  assert.match(card, /if \(previousUrl\) URL\.revokeObjectURL\(previousUrl\)/);
  assert.match(card, /if \(image\) image\.removeAttribute\("src"\)/);
  assert.match(card, /if \(!streaming \|\| !streamView\)/);
});

test("the tile keeps its reserved hint slot and mono facts row", () => {
  assert.match(card, /station-tile-facts/);
  assert.match(cardsStyles, /\.station-tile-facts/);
  assert.match(cardsStyles, /font-variant-numeric: tabular-nums/);
});

test("robot telemetry starts visible and metadata shares one closed disclosure", async () => {
  const body = await readFile(new URL("../src/astribot/pages/StationBodyStatePanel.tsx", import.meta.url), "utf8");
  const models = await readFile(new URL("../src/astribot/pages/RobotModels.tsx", import.meta.url), "utf8");
  assert.match(panel, /className="station-detail-robot"[\s\S]*?<NodeCameras[\s\S]*?<StationBodyStatePanel/);
  assert.match(panel, /<StationBodyStatePanel[^>]*\/>\}[\s\S]*?<details className="station-detail-metadata">[\s\S]*?<div className="station-detail-columns">[\s\S]*?<NodeLinks[\s\S]*?<\/details>/);
  assert.match(body, /<section className="station-body"/);
  assert.doesNotMatch(body, /<details|onToggle/);
  assert.match(body, /<table\s+className="station-body-table station-body-poses"[\s\S]*?<table\s+className="station-body-table station-body-joints"/);
  assert.match(body, /<PoseValues\s+axes=\{positionAxes\}[\s\S]*?<PoseValues\s+axes=\{orientationAxes\}/);
  assert.match(models, /<article className="robot-model-card"[\s\S]*?<header className="robot-model-header">[\s\S]*?<div className="robot-model-capabilities">[\s\S]*?<\/article>/);
});

test("an offline robot offers a bounded re-check and says where the browser stops", () => {
  // The card's recovery line carries the last report and tunnel state (both already on the tile) and the boundary of
  // what re-check can reach; the button runs the page's own load, not a second reader.
  assert.match(stationDisplay, /export function stationRecoveryHint/);
  assert.match(stationDisplay, /现场或走带外接入/);
  assert.match(stationDisplay, /out-of-band path/);
  assert.match(card, /stationRecoveryHint\(station, now, zh\)/);
  assert.match(card, /station-tile-recovery/);
  assert.match(card, /station-tile-recovery-note/);
  assert.match(card, /station-tile-recheck/);
  assert.match(card, /recheck\.pending/);
  assert.match(card, /recheck\.run/);
  assert.match(page, /const \[recheckKey, setRecheckKey\] = useState\(""\)/);
  assert.match(page, /const recheck = async \(item: StationItem\)/);
  assert.match(page, /await load\(\);\s*\} finally \{\s*setRecheckKey\(""\);/);
  assert.match(page, /recheck=\{\{ pending: recheckKey === key, run: actions\.run \}\}/);
  // The open detail repeats the note and offers the same re-read.
  assert.match(panel, /stationRecoveryHint\(station, now, zh\)/);
  assert.match(panel, /station-detail-recovery/);
  assert.match(panel, /重新检测/);
  assert.match(cardsStyles, /\.station-tile-recovery/);
  assert.match(cardsStyles, /\.station-tile-recheck/);
  assert.match(cardsStyles, /\.station-detail-recovery/);
});

test("an offline station offers Rejoin beside Re-check and only opens the join page", () => {
  // The second entry on the recovery line opens the join page for this node; it is not a second re-check.
  assert.match(card, /station-tile-recovery-actions/);
  assert.match(card, /className="station-tile-recheck station-tile-rejoin"/);
  assert.match(card, /重新接入/);
  assert.match(card, /Rejoin/);
  assert.match(card, /onClick=\{rejoin\}/);
  assert.doesNotMatch(card, /rejoin\.pending/, "the card does not run a join or wait on one");
  assert.match(cardsStyles, /\.station-tile-recovery-actions/);
  // The target carries the node's own name and the cluster only when one cluster claims its namespace.
  assert.match(page, /const rejoin = async \(item: StationItem\) =>/);
  assert.match(page, /clusterForStation\(clusters, item\.metadata\.namespace\)/);
  assert.match(page, /nodeName: item\.metadata\.name/);
  // An existing SSH join record wins over a blank form; otherwise the form starts from the mapped login/address.
  assert.match(page, /parseSSHLogin\(admission\?\.ssh_login\)/);
  assert.match(page, /const existing = Boolean\(cluster && \(admission\?\.join_phase \|\| admission\?\.ssh_login\)\)/);
  assert.match(page, /admission\?\.join_phase \|\| admission\?\.ssh_login/);
  assert.match(page, /name: `\$\{cluster\.id\}\.\$\{item\.metadata\.name\}`/);
  assert.match(page, /status\?\.addresses\?\.find\(\(entry\) => entry\.type === "InternalIP"\)\?\.address/);
  assert.match(page, /rejoin: \(\) => void cardHandlers\.current\.rejoin\(item\)/);
  // Opening the page is the whole effect: the admission lookup is read-only and scoped to the station’s cluster.
  const rejoinBody = page.slice(page.indexOf("const rejoin = async (item: StationItem) =>"), page.indexOf("// A whole-machine restart"));
  assert.ok(rejoinBody);
  assert.match(rejoinBody, /nodeAdmissionsApi\.list\(cluster\.id,/);
  assert.doesNotMatch(rejoinBody, /nodeJoinsApi|nodeAdmissionsApi\.(revoke|reissue)|selfHealApi|stationsApi/);
});

test("an online S1U robot offers a confirmed whole-machine restart through the one-shot Node label", () => {
  // The entry exists only for a Bound S1U robot: the tuner that acts on the label is the robot Addon's.
  assert.match(page, /machineModel\(labels\)\.toLowerCase\(\) === "s1u"/);
  assert.match(page, /stationPhase\(item\) === "Bound"/);
  assert.match(page, /const restartPress = \(item: StationItem\)/);
  // The confirmation says someone must be on site and that the tuner's guards still apply.
  assert.match(page, /window\.confirm\(zh/);
  assert.match(page, /现场有人/);
  assert.match(page, /someone is on site/);
  assert.match(page, /急停守卫|emergency-stop guards/);
  // The hold is released first, and only an explicit "released" proceeds to the confirmation and the write; the
  // browser never blocks on the dialog while the machine may still be moving.
  assert.match(page, /releasePoseSessions\(\[\{ key: keyOf\(item\), node: name, namespace: item\.metadata\.namespace, labels: labelsOf\(item\) \}\]\)\s*\.then\(\(release\) => \{[\s\S]*?release !== "released"[\s\S]*?window\.confirm\(zh/);
  assert.match(page, /restart\.reject\(name/);
  assert.match(page, /nodeUID: item\.spec\.nodeUID/);
  // Only the standalone label is written through the existing admin Node PATCH; the claim holder's annotation and
  // command paths are not used.
  assert.match(stationsApi, /requestRestart\(node: string, namespace: string, requestID: string, resourceVersion\?: string\)/);
  assert.match(stationsApi, /\[RESTART_REQUEST_LABEL\]: requestID/);
  assert.match(stationsApi, /resourceVersion/);
  assert.match(stationsApi, /nodesApi\.patch/);
  assert.doesNotMatch(page, /robot-restart/);
  assert.doesNotMatch(page, /wake-on-lan|WakeOnLan/i);
  // A read that passed the outstanding check pins the write: 409 asks for a re-check and is never retried.
  assert.match(restartHook, /resourceVersion = node\.metadata\?\.resourceVersion/);
  assert.match(restartHook, /remained[\s\S]*?requestRestart|requestRestart\(\s*target\.node,\s*target\.namespace,\s*requestID,\s*resourceVersion,?\s*\)/);
  assert.match(restartHook, /ApiError/);
  assert.match(restartHook, /status === 409/);
  assert.match(restartHook, /不会自动重试写入|not retried automatically/);
  // The same UID binding guards the preflight read and every observation.
  assert.match(restartHook, /restartUIDMatches\(target\.nodeUID, node\.status\?\.kubernetesNodeUID\)/);
  assert.match(restartHook, /readable && !restartUIDMatches\(target\.nodeUID, observedUID\)/);
  // An older, untracked request on the Node is visible and blocks a second submission.
  assert.match(restartLib, /export function outstandingRestart/);
  assert.match(page, /outstandingRestart\(observation\)/);
  assert.match(page, /restartBlocked/);
  assert.match(page, /restart\.requests\[key\]/);
  // The card shows the state and the control group; the request is followed on the Node by name on a bounded series.
  assert.match(card, /station-tile-restart/);
  assert.match(card, /station-tile-action-group is-restart/);
  assert.match(card, /整机重启|Restart/);
  assert.match(card, /is-restart/);
  assert.match(restartHook, /RESTART_OBSERVE_INTERVAL_MS = 5000/);
  assert.match(restartHook, /RESTART_WINDOW_MS = 8 \* 60 \* 1000/);
  assert.match(restartHook, /nodesApi\.get\(target\.node, \{ namespace: target\.namespace \}\)/);
  assert.match(restartLib, /rlark\.io\/restart-request-state/);
  assert.match(restartLib, /rlark\.io\/restart-request/);
  assert.match(cardsStyles, /\.station-tile-restart/);
  assert.match(cardsStyles, /station-tile-action-group\.is-restart/);
  // The page-level toast reports the Gateway's answer, like the power one.
  assert.match(page, /restart\.activeLabel/);
  assert.match(page, /restart\.failed/);
  assert.match(page, /restart\.clear/);
});

test("restart records tell submitted, waiting, restarting, done and failed apart", async () => {
  const { readNodeRestart, restartPhaseOf, outstandingRestart, restartUIDMatches, restartWhy, newRestartRequestID } = await import("../dist/test/astribot/lib/nodeRestart.js");
  const label = { "rlark.io/restart-request": "ui-abc" };
  // Written, not picked up yet: outstanding, and a second request must not overwrite it.
  let observation = readNodeRestart(label, {});
  assert.equal(restartPhaseOf(observation, "ui-abc"), "");
  assert.deepEqual(outstandingRestart(observation), { request: "ui-abc", phase: "submitted", why: "" });
  // The tuner's guard: it has the request but a reason holds it.
  observation = readNodeRestart(label, { "rlark.io/maintenance": JSON.stringify({ of: "request ui-abc", state: "waiting", why: "Run run-1 holds the robot" }) });
  assert.equal(restartPhaseOf(observation, "ui-abc"), "waiting");
  assert.match(outstandingRestart(observation).why, /Run run-1/);
  // Started: the record names the request while the host is still coming back.
  observation = readNodeRestart(label, { "rlark.io/restart-request-state": JSON.stringify({ kind: "host", profile: "ui-abc", attempts: 1, was: "6.8.0" }) });
  assert.equal(restartPhaseOf(observation, "ui-abc"), "acting");
  // Done: the boot that followed is recorded, and the label is then history so a new id may be asked for.
  observation = readNodeRestart(label, { "rlark.io/restart-request-state": JSON.stringify({ kind: "host", profile: "ui-abc", done: "2026-11-27T10:00:00Z" }) });
  assert.equal(restartPhaseOf(observation, "ui-abc"), "done");
  assert.equal(outstandingRestart(observation), undefined);
  // Failure the tuner reports against the request.
  observation = readNodeRestart(label, { "rlark.io/maintenance": JSON.stringify({ of: "request ui-abc", state: "failed", why: "the robot did not come down" }) });
  assert.equal(restartPhaseOf(observation, "ui-abc"), "failed");
  assert.match(restartWhy(observation, "ui-abc"), /did not come down/);
  assert.equal(outstandingRestart(observation), undefined, "a failed value is not outstanding");
  // A refusal against the node profile is a failure with its reason, and is not retried for the same value.
  observation = readNodeRestart(label, { "rlark.io/node-profile-report": "restart request ui-abc refused, the node profile does not hold; reboot-required" });
  assert.equal(restartPhaseOf(observation, "ui-abc"), "failed");
  assert.equal(outstandingRestart(observation), undefined, "a refused value is not outstanding");
  // Records of another request do not answer for this one.
  assert.equal(restartPhaseOf(readNodeRestart({ "rlark.io/restart-request": "ui-abc" }, { "rlark.io/restart-request-state": JSON.stringify({ profile: "other", done: "2026-11-27T10:00:00Z" }) }), "ui-abc"), "");
  // A fresh id every time, valid as a Kubernetes label value.
  const id = newRestartRequestID(1_700_000_000_000, () => 0.5);
  assert.match(id, /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/);
  assert.ok(id.length <= 63);
  assert.notEqual(newRestartRequestID(1_700_000_000_000, () => 0.25), newRestartRequestID(1_700_000_000_000, () => 0.75));
  // Identity: a missing or different UID is never the bound machine.
  assert.equal(restartUIDMatches("uid-a", "uid-a"), true);
  assert.equal(restartUIDMatches("uid-a", "uid-b"), false);
  assert.equal(restartUIDMatches("", "uid-a"), false);
});

test("fleet views share KPI filters and select across all loaded pages", () => {
  assert.match(page, /stationsApi\.summary\(clusterID/);
  assert.match(page, /computeFleetSummary\(fleet, clusters\)/);
  assert.match(page, /setSummaryFilter/);
  assert.match(page, /defaultViewModeForCount/);
  assert.match(page, /localStorage\.setItem\(VIEW_STORAGE_KEY, mode\)/);
  assert.match(page, /visibleRobots\.map\(selectionKey\)/);
  assert.match(page, /while \(next\)/);
  assert.match(page, /const admissionsScope = showAdmissions \? "\*" : detailClusterID/);
});

test("only virtual rows mount and live connections use shared view budgets and dialog priority", async () => {
  const table = await readFile(new URL("../src/astribot/pages/StationsTableView.tsx", import.meta.url), "utf8");
  const wall = await readFile(new URL("../src/astribot/pages/CameraWallView.tsx", import.meta.url), "utf8");
  assert.match(table, /computeVirtualWindow\([\s\S]*?overscan: 8/);
  assert.match(table, /items\.slice\(startIndex, endIndex\)/);
  assert.doesNotMatch(table, /cameraStreamPath|useCameraClock/);
  assert.match(card, /memo\(function StationCard/);
  assert.match(card, /cameraMediaScheduler\.acquire\(leaseID/);
  assert.match(card, /cameraMediaScheduler\.promote\(leaseID, "pinned"\)/);
  assert.match(card, /useCameraClock\(node, namespace, streaming\)/);
  assert.match(wall, /size = columns \* columns/);
  assert.match(wall, /setMaxLiveStreams\(size\)/);
  assert.match(wall, /setMaxLiveStreams\(3\)/);
  assert.match(dialog, /cameraMediaScheduler\.acquire\(.*"dialog", setLiveGranted\)/);
});

test("the batch drawer shows failed robots and forwards cancel, retry and dismiss", async () => {
  const batchPanel = await readFile(new URL("../src/astribot/components/BatchProgressPanel.tsx", import.meta.url), "utf8");
  assert.match(page, /onCoalescedRefresh:/);
  assert.match(page, /<BatchProgressPanel batch=\{power\.batch\}/);
  assert.match(batchPanel, /item\.status === "failed"/);
  assert.match(batchPanel, /item\.error/);
  assert.match(batchPanel, /onClick=\{cancelBatch\}/);
  assert.match(batchPanel, /void retryFailed\(\)/);
  assert.match(batchPanel, /onClick=\{dismissBatch\}/);
});
