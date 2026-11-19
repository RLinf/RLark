#!/usr/bin/env python3

import glob
import os
import re
import sys
import xml.etree.ElementTree as ET


POSITION = re.compile(r"^(.+):(\d+)\.\d+,(\d+)\.\d+ \d+ (\d+)$")


def source_path(profile, filename):
    module = os.path.basename(profile).removesuffix(".out").replace("apps-", "apps/", 1)
    filename = filename.removeprefix("github.com/rlinf/rlark/")
    if filename != module and not filename.startswith(f"{module}/"):
        filename = f"{module}/{filename}"
    return filename


def main():
    profiles = glob.glob("coverage/go/*.out")
    if not profiles:
        sys.exit("no Go coverage profiles found")

    covered = {}
    for profile in profiles:
        with open(profile, encoding="utf-8") as source:
            for row in source:
                row = row.strip()
                if not row or row.startswith("mode:"):
                    continue
                match = POSITION.match(row)
                if not match:
                    sys.exit(f"invalid coverage row in {profile}: {row}")
                filename = source_path(profile, match.group(1))
                lines = covered.setdefault(filename, {})
                hit = int(match.group(4)) > 0
                for number in range(int(match.group(2)), int(match.group(3)) + 1):
                    lines[number] = lines.get(number, False) or hit

    total = sum(len(lines) for lines in covered.values())
    hits = sum(sum(lines.values()) for lines in covered.values())
    rate = hits / total if total else 0
    report = ET.Element(
        "coverage",
        {
            "line-rate": str(rate),
            "lines-covered": str(hits),
            "lines-valid": str(total),
            "version": "rlark",
        },
    )
    ET.SubElement(ET.SubElement(report, "sources"), "source").text = "."
    package = ET.SubElement(
        ET.SubElement(report, "packages"),
        "package",
        {"name": "rlark", "line-rate": str(rate)},
    )
    classes = ET.SubElement(package, "classes")
    for filename, lines in sorted(covered.items()):
        class_rate = sum(lines.values()) / len(lines) if lines else 0
        class_element = ET.SubElement(
            classes,
            "class",
            {"name": filename, "filename": filename, "line-rate": str(class_rate)},
        )
        ET.SubElement(class_element, "methods")
        line_elements = ET.SubElement(class_element, "lines")
        for number, hit in sorted(lines.items()):
            ET.SubElement(line_elements, "line", {"number": str(number), "hits": str(int(hit))})

    os.makedirs("coverage", exist_ok=True)
    ET.ElementTree(report).write("coverage/cobertura.xml", encoding="utf-8", xml_declaration=True)
    print(f"TOTAL COVERAGE: {rate * 100:.2f}% ({hits}/{total} lines)")


if __name__ == "__main__":
    main()
