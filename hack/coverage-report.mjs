import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const coverage = new Map();

function add(file, line, covered) {
  const relative = path.relative(root, path.resolve(root, file)).replaceAll(path.sep, "/");
  if (relative.startsWith("../")) return;
  const lines = coverage.get(relative) ?? new Map();
  lines.set(line, (lines.get(line) ?? 0) + (covered ? 1 : 0));
  coverage.set(relative, lines);
}

function readV8(directory) {
  if (!fs.existsSync(directory)) return;
  for (const name of fs.readdirSync(directory).filter((file) => file.endsWith(".json"))) {
    const report = JSON.parse(fs.readFileSync(path.join(directory, name), "utf8"));
    for (const script of report.result ?? []) {
      if (!script.url.startsWith("file:")) continue;
      const file = fileURLToPath(script.url);
      if (!file.includes(`${path.sep}apps${path.sep}rlark-ui${path.sep}dist${path.sep}test${path.sep}`)) continue;
      const source = fs.readFileSync(file, "utf8");
      const offsets = [0];
      for (let index = 0; index < source.length; index++) if (source[index] === "\n") offsets.push(index + 1);
      const lineAt = (offset) => {
        let low = 0;
        let high = offsets.length;
        while (low + 1 < high) {
          const middle = Math.floor((low + high) / 2);
          if (offsets[middle] <= offset) low = middle;
          else high = middle;
        }
        return low + 1;
      };
      const output = path.relative(path.join(root, "apps/rlark-ui/dist/test"), file).replace(/\.js$/, ".ts");
      const target = `apps/rlark-ui/src/utils/${output}`;
      const lineHits = new Map();
      for (const fn of script.functions ?? []) {
        for (const range of fn.ranges ?? []) {
          for (let line = lineAt(range.startOffset); line <= lineAt(Math.max(range.startOffset, range.endOffset - 1)); line++) {
            const current = lineHits.get(line);
            lineHits.set(line, current === undefined ? range.count : Math.min(current, range.count));
          }
        }
      }
      for (const [line, count] of lineHits) add(target, line, count > 0);
    }
  }
}

function xml(value) {
  return value.replaceAll("&", "&amp;").replaceAll('"', "&quot;").replaceAll("<", "&lt;");
}

readV8(path.join(root, "apps/rlark-ui/coverage/v8"));

let total = 0;
let hit = 0;
const classes = [];
for (const [file, lines] of [...coverage].sort()) {
  for (const count of lines.values()) {
    total++;
    if (count > 0) hit++;
  }
  const entries = [...lines].sort((a, b) => a[0] - b[0]);
  classes.push(`      <class name="${xml(file)}" filename="${xml(file)}" line-rate="${entries.filter(([, count]) => count > 0).length / entries.length}"><methods/><lines>${entries.map(([line, count]) => `<line number="${line}" hits="${count}"/>`).join("")}</lines></class>`);
}
const rate = total ? hit / total : 0;
fs.mkdirSync(path.join(root, "coverage"), { recursive: true });
fs.writeFileSync(path.join(root, "coverage/cobertura.xml"), `<?xml version="1.0"?><coverage line-rate="${rate}" lines-covered="${hit}" lines-valid="${total}" version="rlark"><sources><source>.</source></sources><packages><package name="rlark" line-rate="${rate}"><classes>\n${classes.join("\n")}\n</classes></package></packages></coverage>\n`);
console.log(`TOTAL COVERAGE: ${(rate * 100).toFixed(2)}% (${hit}/${total} lines)`);
