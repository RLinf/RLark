"""Patch mkdocs.yml and zh markdown files for ReadTheDocs builds."""
import re
import os
import glob

lang = os.environ.get("READTHEDOCS_LANGUAGE", "en")
# Normalize: RTD uses "zh-cn" but i18n plugin expects "zh"
if lang == "zh-cn":
    lang = "zh"

config_path = "apps/rlark/mkdocs.yml"

with open(config_path) as f:
    content = f.read()

# Inject build_only_locale
content = re.sub(
    r"(docs_structure: folder\n)",
    rf"\1      build_only_locale: {lang}\n",
    content,
)

# Inject language switcher (extra.alternate) for cross-project links on RTD
# RTD uses "zh-cn" for Chinese translation subproject URL
version = os.environ.get("READTHEDOCS_VERSION", "latest")
alternate_block = f"""
extra:
  alternate:
    - name: English
      link: https://rlark.readthedocs.io/en/{version}/
      lang: en
    - name: 中文
      link: https://rlark.readthedocs.io/zh-cn/{version}/
      lang: zh-cn
"""

content = re.sub(r"\n# .*navigation\.instant.*\n", "\n", content)
content = content.rstrip() + "\n" + alternate_block

with open(config_path, "w") as f:
    f.write(content)

# Fix image paths in zh/ markdown files
# When build_only_locale=zh, pages are at root, but markdown assumes zh/ subdir
# ../../images/ -> ../images/ (for files in zh/subdir/)
# ../images/    -> images/     (for files in zh/ root)
for md_file in glob.glob("apps/rlark/docs/zh/**/*.md", recursive=True):
    with open(md_file) as f:
        md_content = f.read()
    # Remove one level of "../" from image paths
    md_content = md_content.replace("../../images/", "__TEMP_IMAGES__/")
    md_content = md_content.replace("../images/", "images/")
    md_content = md_content.replace("__TEMP_IMAGES__/", "../images/")
    with open(md_file, "w") as f:
        f.write(md_content)

print(f"[i18n] build_only_locale={lang}")