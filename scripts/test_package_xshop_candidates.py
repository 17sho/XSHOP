"""CLI packaging regressions with explicitly synthetic binaries, not deployment evidence."""
import pathlib,tempfile,subprocess,unittest,json,hashlib
SCRIPT=pathlib.Path(__file__).with_name('package_xshop_candidates.py')
class Packaging(unittest.TestCase):
 def setUp(self):
  self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup);self.base=pathlib.Path(self.tmp.name);self.root=self.base/'repo';self.root.mkdir();self.out=self.base/'output';(self.out/'A').mkdir(parents=True);(self.out/'B').mkdir();(self.out/'A/dujiao-next').write_bytes(b'synthetic-A');(self.out/'B/dujiao-next').write_bytes(b'synthetic-B')
  (self.root/'README.md').write_text('Synthetic packaging fixture.\n');subprocess.run(['git','init','-q',str(self.root)],check=True);subprocess.run(['git','-C',str(self.root),'add','.'],check=True);subprocess.run(['git','-C',str(self.root),'-c','user.name=Synthetic Fixture','-c','user.email=fixture@example.invalid','commit','-qm','Synthetic packaging fixture'],check=True)
  self.key=self.base/'key.pem';subprocess.run(['openssl','genpkey','-algorithm','ED25519','-out',str(self.key)],check=True,capture_output=True)
 def call(self,*args):return subprocess.run(['python3',str(SCRIPT),str(self.root),str(self.out),str(self.key),*args],text=True,capture_output=True)
 def test_next_release_does_not_reuse_consumed_sequence(self):
  p=self.call('--version','xshop-preview-b2','--sequence','3');self.assertEqual(p.returncode,0,p.stderr);m=json.loads((self.out/'B/manifest.json').read_text());self.assertEqual(m['sequence'],3);self.assertEqual(m['version'],'xshop-preview-b2');self.assertEqual(m['archive']['name'],'xshop-preview-b2-linux-amd64.tar.gz');self.assertEqual(m['source']['name'],'xshop-preview-b2-source.tar.gz');self.assertEqual(m['from_binary_sha256'],[hashlib.sha256(b'synthetic-A').hexdigest()]);self.assertTrue((self.out/'B/manifest.sig').exists())
 def test_original_defaults_preserved(self):
  p=self.call();self.assertEqual(p.returncode,0,p.stderr);m=json.loads((self.out/'B/manifest.json').read_text());self.assertEqual(m['version'],'xshop-preview-b1');self.assertEqual(m['sequence'],2)
 def test_invalid_release_identity_rejected_before_writes(self):
  for value in ['../escape','other-product','xshop-preview-../escape']:
   with self.subTest(value=value):self.assertNotEqual(self.call('--version',value,'--sequence','3').returncode,0);self.assertFalse((self.out/'B/manifest.json').exists())
 def test_invalid_sequence_rejected(self):
  for value in ['0','1','-1','18446744073709551616']:
   with self.subTest(value=value):self.assertNotEqual(self.call('--version','xshop-preview-b2','--sequence',value).returncode,0);self.assertFalse((self.out/'B/manifest.json').exists())
if __name__=='__main__':unittest.main(verbosity=2)
