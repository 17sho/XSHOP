import re
import subprocess
import tempfile
import unittest
from pathlib import Path

from scripts.check_release_source import check_release_source


class ReleaseSourceTests(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.repo = Path(self.directory.name)
        subprocess.run(['git', 'init', '-q', str(self.repo)], check=True)
        subprocess.run(['git', '-C', str(self.repo), 'config', 'user.email', 'test@example.invalid'], check=True)
        subprocess.run(['git', '-C', str(self.repo), 'config', 'user.name', 'Test'], check=True)
        (self.repo / 'source.txt').write_text('baseline')
        subprocess.run(['git', '-C', str(self.repo), 'add', 'source.txt'], check=True)
        subprocess.run(['git', '-C', str(self.repo), 'commit', '-qm', 'baseline'], check=True)

    def test_accepts_clean_repository(self):
        self.assertEqual(check_release_source(self.repo), [])

    def test_rejects_modified_tracked_source(self):
        (self.repo / 'source.txt').write_text('changed')
        self.assertIn('uncommitted', str(check_release_source(self.repo)))

    def test_rejects_untracked_source(self):
        (self.repo / 'new.txt').write_text('new')
        self.assertIn('untracked', str(check_release_source(self.repo)))

    def test_rejects_ignored_source_even_with_local_exclude(self):
        (self.repo / 'frontend' / 'user' / 'src').mkdir(parents=True)
        (self.repo / '.git' / 'info' / 'exclude').write_text('frontend/user/src/hidden.ts\n')
        (self.repo / 'frontend' / 'user' / 'src' / 'hidden.ts').write_text('export const hidden = true')
        self.assertIn('ignored source', str(check_release_source(self.repo)))

    def test_rejects_ignored_public_asset(self):
        (self.repo / 'frontend' / 'user' / 'public').mkdir(parents=True)
        (self.repo / '.git' / 'info' / 'exclude').write_text('frontend/user/public/logo.png\n')
        (self.repo / 'frontend' / 'user' / 'public' / 'logo.png').write_bytes(b'image')
        self.assertIn('ignored build input', str(check_release_source(self.repo)))

    def test_rejects_ignored_frontend_config(self):
        (self.repo / 'frontend' / 'admin').mkdir(parents=True)
        (self.repo / '.git' / 'info' / 'exclude').write_text('frontend/admin/vite.config.ts\n')
        (self.repo / 'frontend' / 'admin' / 'vite.config.ts').write_text('export default {}')
        self.assertIn('ignored build input', str(check_release_source(self.repo)))

    def test_rejects_ignored_index_and_stylesheet(self):
        (self.repo / 'frontend' / 'user' / 'src').mkdir(parents=True)
        (self.repo / '.git' / 'info' / 'exclude').write_text('frontend/user/index.html\nfrontend/user/src/theme.css\n')
        (self.repo / 'frontend' / 'user' / 'index.html').write_text('<div id="app"></div>')
        (self.repo / 'frontend' / 'user' / 'src' / 'theme.css').write_text('body { color: red }')
        self.assertIn('ignored build input', str(check_release_source(self.repo)))
        self.assertIn('ignored source', str(check_release_source(self.repo)))

    def test_python_check_does_not_dirty_a_clean_checkout(self):
        (self.repo / 'scripts').mkdir()
        (self.repo / 'scripts' / 'check_release_source.py').write_bytes(
            Path(__file__).resolve().parents[1].joinpath('check_release_source.py').read_bytes()
        )
        (self.repo / '.gitignore').write_text('__pycache__/\n*.pyc\n')
        subprocess.run(['git', '-C', str(self.repo), 'add', '.gitignore', 'scripts'], check=True)
        subprocess.run(['git', '-C', str(self.repo), 'commit', '-qm', 'add release gate'], check=True)
        result = subprocess.run(['python3', str(self.repo / 'scripts' / 'check_release_source.py')],
                                cwd=self.repo, capture_output=True, text=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(check_release_source(self.repo), [])

    def test_ignores_build_output_only_when_git_ignores_it(self):
        (self.repo / '.gitignore').write_text('dist/\n')
        subprocess.run(['git', '-C', str(self.repo), 'add', '.gitignore'], check=True)
        subprocess.run(['git', '-C', str(self.repo), 'commit', '-qm', 'ignore output'], check=True)
        (self.repo / 'dist').mkdir()
        (self.repo / 'dist' / 'index.html').write_text('generated')
        self.assertEqual(check_release_source(self.repo), [])


class FrontendWorkflowOrderTests(unittest.TestCase):
    def test_clean_checkout_builds_before_dist_dependent_tests(self):
        root = Path(__file__).resolve().parents[2]
        for workflow in ('ci.yml', 'release.yml'):
            with self.subTest(workflow=workflow):
                source = (root / '.github' / 'workflows' / workflow).read_text()
                if workflow == 'ci.yml':
                    source = source.split('\n  fullstack:', 1)[1]
                # Ignore comments: documentation is not an executable gate.
                source = '\n'.join(line for line in source.splitlines()
                                   if not line.lstrip().startswith('#'))

                def position(pattern):
                    match = re.search(pattern, source)
                    if match is None:
                        self.fail(f'{workflow}: missing gate {pattern}')
                    return match.start()

                stages = [
                    ('frozen installs', [
                        position(rf'cd frontend/{app}\s*&& pnpm install --frozen-lockfile')
                        for app in ('admin', 'user')]),
                    ('dependency behavior gates', [
                        position(rf'node scripts/check_frontend_dependency_safety\.cjs {app}')
                        for app in ('admin', 'user')]),
                    ('typechecked frontend builds', [
                        position(r'cd frontend/admin\s*&& pnpm run build:fullstack'),
                        position(r'cd frontend/user\s*&& pnpm run build\b')]),
                    ('dist-dependent frontend tests', [
                        position(rf'cd frontend/{app}\b[^\n]*?pnpm (?:run )?test\b')
                        for app in ('admin', 'user')]),
                    ('built asset verification', [position(
                        r'python3 scripts/check_frontend_assets\.py frontend/admin/dist frontend/user/dist')]),
                    ('copy verified assets for embedding', [
                        position(rf'cp -r frontend/{app}/dist\s+internal/web/dist/{app}')
                        for app in ('admin', 'user')]),
                    ('final fullstack Go build', [position(r'go build -tags release,fullstack')]),
                ]
                for (before, earlier), (after, later) in zip(stages, stages[1:]):
                    self.assertLess(max(earlier), min(later),
                                    f'{workflow}: {before} must precede {after}')


if __name__ == '__main__':
    unittest.main()
