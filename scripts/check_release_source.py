"""Reject a release build unless all source comes from one clean Git commit."""

import subprocess
import sys
from pathlib import Path


def check_release_source(root: Path) -> list[str]:
    root = Path(root)
    result = subprocess.run(
        ['git', '-C', str(root), 'status', '--porcelain', '--untracked-files=normal'],
        capture_output=True,
        text=True,
        check=False,
    )
    if result.returncode:
        return [f'{root}: cannot verify Git source state: {result.stderr.strip()}']
    changes = result.stdout.splitlines()
    errors = []
    if changes:
        tracked = sum(not line.startswith('??') for line in changes)
        untracked = len(changes) - tracked
        errors.append(f'{root}: release requires a clean Git source (uncommitted: {tracked}, untracked: {untracked})')
    # Git excludes are local metadata, not release inputs. Reject ignored
    # source, public assets, and frontend build configuration outside dist.
    for directory, suffixes, label in (
        ('frontend/user/src', ('.ts', '.tsx', '.vue', '.js', '.jsx', '.css', '.scss', '.json', '.svg'), 'ignored source'),
        ('frontend/admin/src', ('.ts', '.tsx', '.vue', '.js', '.jsx', '.css', '.scss', '.json', '.svg'), 'ignored source'),
        ('internal', ('.go',), 'ignored source'),
        ('cmd', ('.go',), 'ignored source'),
        ('frontend/user/public', None, 'ignored build input'),
        ('frontend/admin/public', None, 'ignored build input'),
        ('frontend/user', ('index.html', 'vite.config.ts', 'package.json', 'pnpm-lock.yaml', 'tsconfig.json'), 'ignored build input'),
        ('frontend/admin', ('index.html', 'vite.config.ts', 'package.json', 'pnpm-lock.yaml', 'tsconfig.json'), 'ignored build input'),
    ):
        path = root / directory
        if not path.exists():
            continue
        ignored = subprocess.run(
            ['git', '-C', str(root), 'ls-files', '--others', '--ignored', '--exclude-standard', '--', directory],
            capture_output=True, text=True, check=False,
        )
        if ignored.returncode:
            errors.append(f'{root}: cannot verify ignored source in {directory}')
            continue
        names = [name for name in ignored.stdout.splitlines()
                 if (suffixes is None or name.endswith(suffixes))
                 and (label == 'ignored source' or label == 'ignored build input'
                      and (directory.endswith('/public') or name.count('/') == directory.count('/') + 1))]
        if names:
            errors.append(f'{root}: {label} under {directory}: {len(names)}')
    return errors


def main(argv: list[str]) -> int:
    if len(argv) > 1:
        print('usage: check_release_source.py [REPOSITORY_ROOT]', file=sys.stderr)
        return 2
    root = Path(argv[0]) if argv else Path(__file__).resolve().parent.parent
    errors = check_release_source(root)
    for error in errors:
        print(error, file=sys.stderr)
    if errors:
        return 1
    revision = subprocess.run(
        ['git', '-C', str(root), 'rev-parse', 'HEAD'],
        capture_output=True,
        text=True,
        check=True,
    ).stdout.strip()
    print(f'release source: {revision}')
    return 0


if __name__ == '__main__':
    raise SystemExit(main(sys.argv[1:]))
