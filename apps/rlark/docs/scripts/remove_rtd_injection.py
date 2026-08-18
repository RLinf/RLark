"""Remove RTD's JavaScript injection from built HTML pages.

Read the Docs injects its own JavaScript and CSS into the built HTML files,
which conflicts with the mkdocs-material theme (navigation sidebar, dark/light
mode toggle, etc.). This script removes those injected elements.
"""

import os
import re

output_dir = os.environ.get("READTHEDOCS_OUTPUT", "site")
html_dir = os.path.join(output_dir, "html")

for root, dirs, files in os.walk(html_dir):
    for f in files:
        if f.endswith(".html"):
            path = os.path.join(root, f)
            with open(path) as fh:
                content = fh.read()
            # Remove RTD's injected script and link tags
            content = re.sub(
                r'<script[^>]*readthedocs[^>]*></script>', "", content
            )
            content = re.sub(r'<link[^>]*readthedocs[^>]*>', "", content)
            with open(path, "w") as fh:
                fh.write(content)