"""Patch mkdocs.yml for Read the Docs locale builds."""

import os
import re
from pathlib import Path
from urllib.parse import urlsplit, urlunsplit


lang = os.environ.get("READTHEDOCS_LANGUAGE", "en")
if lang == "zh-cn":
    lang = "zh"

config_path = "apps/rlark/mkdocs.yml"

with open(config_path, encoding="utf-8") as config_file:
    content = config_file.read()

content, count = re.subn(
    r"(docs_structure: folder\n)(?:\s+build_only_locale:.*\n)?",
    rf"\1      build_only_locale: {lang}\n",
    content,
    count=1,
)
if count != 1:
    raise RuntimeError("could not locate the i18n docs_structure setting")

content = re.sub(
    r"\n# BEGIN RTD ALTERNATES\n.*?\n# END RTD ALTERNATES\n?",
    "\n",
    content,
    flags=re.DOTALL,
)

canonical_url = os.environ.get("READTHEDOCS_CANONICAL_URL", "")
parsed_url = urlsplit(canonical_url)
path_parts = [part for part in parsed_url.path.split("/") if part]
project_root = None
if parsed_url.scheme in {"http", "https"} and parsed_url.netloc and len(path_parts) >= 2:
    locale = path_parts[-2]
    if locale in {"en", "zh", "zh-cn"}:
        root_path = "/".join(path_parts[:-2])
        project_root = urlunsplit(
            (parsed_url.scheme, parsed_url.netloc, f"/{root_path}" if root_path else "", "", "")
        ).rstrip("/")

if project_root:
    version = os.environ.get("READTHEDOCS_VERSION", "latest")
    alternate_block = f"""
# BEGIN RTD ALTERNATES
extra:
  alternate:
    - name: English
      link: {project_root}/en/{version}/
      lang: en
    - name: 中文
      link: {project_root}/zh-cn/{version}/
      lang: zh-cn
# END RTD ALTERNATES
"""
    content = content.rstrip() + "\n" + alternate_block

with open(config_path, "w", encoding="utf-8") as config_file:
    config_file.write(content)

if lang == "zh":
    zh_docs = Path("apps/rlark/docs/zh")
    for doc_path in zh_docs.rglob("*.md"):
        relative_depth = len(doc_path.relative_to(zh_docs).parent.parts)
        source_prefix = "../" * (relative_depth + 1) + "images/"
        output_prefix = "../" * relative_depth + "images/"

        doc_content = doc_path.read_text(encoding="utf-8")
        patched_content = re.sub(
            rf"(?P<attribute>\bsrc\s*=\s*[\"']){re.escape(source_prefix)}",
            rf"\g<attribute>{output_prefix}",
            doc_content,
        )
        if patched_content != doc_content:
            doc_path.write_text(patched_content, encoding="utf-8")

print(f"[i18n] build_only_locale={lang}")
