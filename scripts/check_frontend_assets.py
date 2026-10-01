"""Check that a built Vite frontend has its entry and lazy assets.

Run: python3 scripts/check_frontend_assets.py frontend/user/dist frontend/admin/dist
"""

import re
import sys
from pathlib import Path

ENTRY = re.compile(r'''(?:src|href)=["'](?:/|\./)?assets/([\w.-]+\.(?:js|css))["']''')
ROOT_ASSET = re.compile(r'''(?:src|href)=["']/([\w.-]+\.(?:png|jpe?g|svg|webp|gif|woff2?|ttf|ico|webmanifest))["']''')
ASSET = re.compile(r'''(?:\bimport\s*(?:[^;]*?\sfrom\s*)?|\bexport\s+[^;]*?\sfrom\s*|\bimport\s*\(|[\[:,]\s*)["'](?:\./|/?assets/)([\w.-]+\.(?:js|css))["']''')
CSS_URL = re.compile(r'''url\(\s*["']?(?:\./|/assets/)([\w.-]+\.(?:png|jpe?g|svg|webp|gif|woff2?|ttf|ico))''')
JS_URL = re.compile(r'''new URL\(["'](?:\./|/assets/)([\w.-]+\.(?:png|jpe?g|svg|webp|gif|woff2?|ttf|ico))["']\s*,\s*import\.meta\.url\)''')


def check_assets(root: Path, root_assets: Path | None = None) -> list[str]:
    """Return missing/invalid asset references in a built frontend directory."""
    root = Path(root)
    root_assets = Path(root_assets) if root_assets is not None else root
    index = root / 'index.html'
    if not index.is_file():
        return [f'{root}: missing index.html']
    html = index.read_text(encoding='utf-8')
    entries = ENTRY.findall(html)
    if not any(name.endswith('.js') for name in entries):
        return [f'{root}: index.html has no JS entry asset']
    errors = [f'{root}: missing {name}' for name in ROOT_ASSET.findall(html)
              if not (root / name).is_file() and not (root_assets / name).is_file()]
    assets = root / 'assets'
    pending = list(entries)
    visited = set()
    while pending:
        name = pending.pop()
        if name in visited:
            continue
        visited.add(name)
        path = assets / name
        if not path.is_file():
            errors.append(f'{root}: missing assets/{name}')
            continue
        if name.endswith('.js'):
            content = path.read_text(encoding='utf-8')
            pending.extend(ASSET.findall(content))
            pending.extend(JS_URL.findall(content))
        elif name.endswith('.css'):
            pending.extend(CSS_URL.findall(path.read_text(encoding='utf-8')))
    return sorted(errors)


def main(argv: list[str]) -> int:
    if not argv:
        print('usage: check_frontend_assets.py DIST_DIR [DIST_DIR ...]', file=sys.stderr)
        return 2
    errors = []
    # In the fullstack binary, /favicon-v5.svg is served by the user SPA
    # even when referenced from the admin HTML. Preserve that routing here.
    user_root = next((Path(arg) for arg in argv if Path(arg).name in {'user', 'user-dist', 'dist'} and 'user' in Path(arg).parts), None)
    for arg in argv:
        root = Path(arg)
        fallback = user_root if 'admin' in root.parts and user_root else None
        problems = check_assets(root, root_assets=fallback)
        errors.extend(problems)
        if not problems:
            print(f'{root}: entry and lazy assets present')
    for error in errors:
        print(error, file=sys.stderr)
    return 1 if errors else 0


if __name__ == '__main__':
    raise SystemExit(main(sys.argv[1:]))
