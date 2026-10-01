import tempfile
import unittest
from pathlib import Path

from scripts.check_frontend_assets import check_assets


class FrontendAssetTests(unittest.TestCase):
    def test_accepts_entry_and_lazy_chunks(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'assets').mkdir()
            (root / 'index.html').write_text('<script src="/assets/index-A.js"></script>')
            (root / 'assets/index-A.js').write_text('import("./Checkout-B.js")')
            (root / 'assets/Checkout-B.js').write_text('const page = true')
            self.assertEqual(check_assets(root), [])

    def test_rejects_missing_lazy_chunk(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'assets').mkdir()
            (root / 'index.html').write_text('<script src="/assets/index-A.js"></script>')
            (root / 'assets/index-A.js').write_text('import("./Checkout-B.js")')
            self.assertIn('Checkout-B.js', str(check_assets(root)))

    def test_rejects_missing_entry_and_css_reference(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'assets').mkdir()
            (root / 'index.html').write_text('<script src="./assets/index-A.js"></script>')
            (root / 'assets/index-A.js').write_text('import "./panel-B.css"')
            self.assertIn('panel-B.css', str(check_assets(root)))
            (root / 'assets/index-A.js').unlink()
            self.assertIn('index-A.js', str(check_assets(root)))
    def test_rejects_missing_root_icon_and_manifest(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'assets').mkdir()
            (root / 'index.html').write_text('<link rel="icon" href="/favicon.svg"><link rel="manifest" href="/site.webmanifest"><script src="/assets/index-A.js"></script>')
            (root / 'assets/index-A.js').write_text('const app = true')
            problems = str(check_assets(root))
            self.assertIn('favicon.svg', problems)
            self.assertIn('site.webmanifest', problems)

    def test_admin_root_icon_can_be_served_by_embedded_user_frontend(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            admin = root / 'admin'
            user = root / 'user'
            (admin / 'assets').mkdir(parents=True)
            user.mkdir()
            (admin / 'index.html').write_text('<link rel="icon" href="/favicon.svg"><script src="./assets/index-A.js"></script>')
            (admin / 'assets/index-A.js').write_text('const app = true')
            (user / 'favicon.svg').write_text('<svg/>')
            self.assertEqual(check_assets(admin, root_assets=user), [])

    def test_rejects_missing_vite_preload_chunk(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'assets').mkdir()
            (root / 'index.html').write_text('<script src="/assets/index-A.js"></script>')
            (root / 'assets/index-A.js').write_text('const deps = ["assets/Lazy-B.js", "assets/Lazy-C.css"]')
            self.assertIn('Lazy-B.js', str(check_assets(root)))
            self.assertIn('Lazy-C.css', str(check_assets(root)))

    def test_does_not_treat_plain_string_as_an_import(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'assets').mkdir()
            (root / 'index.html').write_text('<script src="/assets/index-A.js"></script>')
            (root / 'assets/index-A.js').write_text('const message = "./not-an-import.js"')
            self.assertEqual(check_assets(root), [])

    def test_rejects_missing_css_image(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'assets').mkdir()
            (root / 'index.html').write_text('<script src="/assets/index-A.js"></script>')
            (root / 'assets/index-A.js').write_text('import "./panel-B.css"')
            (root / 'assets/panel-B.css').write_text('.logo{background:url(./missing.png)}')
            self.assertIn('missing.png', str(check_assets(root)))

    def test_rejects_missing_js_asset_url(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / 'assets').mkdir()
            (root / 'index.html').write_text('<script src="/assets/index-A.js"></script>')
            (root / 'assets/index-A.js').write_text('const logo = new URL("./missing.png", import.meta.url)')
            self.assertIn('missing.png', str(check_assets(root)))


if __name__ == '__main__':
    unittest.main()
