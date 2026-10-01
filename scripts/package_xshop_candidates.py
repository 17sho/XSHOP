#!/usr/bin/env python3
"""Package already-built clean-commit A/B binaries; never publish or deploy."""
import argparse, gzip, hashlib, json, pathlib, subprocess, tarfile, re

def sha(data): return hashlib.sha256(data).hexdigest()
def run(*args): return subprocess.check_output(args)
def package(root, output, key, version='xshop-preview-b1', sequence=2):
    if not re.fullmatch(r'xshop-preview-[a-z0-9][a-z0-9._-]{0,100}', version) or '..' in version:
        raise SystemExit('invalid release identity')
    if not 1 < sequence < 2**64:
        raise SystemExit('invalid release sequence')
    root, output, key = map(lambda p: pathlib.Path(p).resolve(), (root, output, key))
    if run('git','-C',str(root),'status','--porcelain').strip(): raise SystemExit('clean committed source required')
    commit=run('git','-C',str(root),'rev-parse','HEAD').decode().strip()
    public=run('openssl','pkey','-in',str(key),'-pubout','-outform','DER')[-32:].hex()
    output.mkdir(parents=True,exist_ok=True)
    source=run('git','-C',str(root),'archive','--format=tar','--prefix=XSHOP/',commit)
    source=gzip.compress(source,mtime=0)
    tracked=run('git','-C',str(root),'ls-files').decode().splitlines()
    schemas=[p for p in tracked if p.endswith('.go') and not p.endswith('_test.go') and (p.startswith('internal/bootstrap/database/') or '/domain/' in p or 'gorm:"' in (root/p).read_text())]
    fingerprint=sha(json.dumps({p:sha((root/p).read_bytes()) for p in schemas},sort_keys=True,separators=(',',':')).encode())
    (output/'public-key.txt').write_text(public+'\n')
    (output/'schema-fingerprint.txt').write_text(fingerprint+'\n')
    a=(output/'A/dujiao-next').read_bytes(); b=(output/'B/dujiao-next').read_bytes()
    h=tarfile.TarInfo('dujiao-next');h.mode=0o755;h.size=len(b);h.uid=h.gid=0;h.mtime=0;h.uname=h.gname=''
    archive=gzip.compress(h.tobuf(format=tarfile.USTAR_FORMAT)+b+b'\0'*((-len(b))%512)+b'\0'*1024,mtime=0)
    release=output/'B';archive_name=version+'-linux-amd64.tar.gz';source_name=version+'-source.tar.gz'
    (release/archive_name).write_bytes(archive);(release/source_name).write_bytes(source)
    manifest={'schema_version':1,'product':'XSHOP','sequence':sequence,'version':version,'source_commit':commit,'channel':'preview','profile':'embedded-preview','os':'linux','arch':'amd64','minimum_updater':1,'from_binary_sha256':[sha(a)],'migration_policy':'unchanged','schema_fingerprint':fingerprint,'archive':{'name':archive_name,'size':len(archive),'sha256':sha(archive)},'source':{'name':source_name,'size':len(source),'sha256':sha(source)},'files':[{'path':'dujiao-next','size':len(b),'sha256':sha(b),'mode':493}]}
    raw=json.dumps(manifest,separators=(',',':'),ensure_ascii=True).encode();(release/'manifest.json').write_bytes(raw)
    subprocess.run(['openssl','pkeyutl','-sign','-rawin','-inkey',str(key),'-in',str(release/'manifest.json'),'-out',str(release/'manifest.sig')],check=True)
    evidence={'commit':commit,'public_key':public,'schema_fingerprint':fingerprint,'schema_paths':schemas,'artifacts':{str(p.relative_to(output)):{'size':p.stat().st_size,'sha256':sha(p.read_bytes())} for p in sorted(output.rglob('*')) if p.is_file() and p.name!='artifact-ledger.json'}}
    (output/'artifact-ledger.json').write_text(json.dumps(evidence,indent=2)+'\n')
    print(json.dumps({'commit':commit,'A':sha(a),'B':sha(b),'manifest':sha(raw),'archive':sha(archive),'source':sha(source)},indent=2))
if __name__=='__main__':
    p=argparse.ArgumentParser();p.add_argument('root');p.add_argument('output');p.add_argument('private_key');p.add_argument('--version',default='xshop-preview-b1');p.add_argument('--sequence',type=int,default=2);a=p.parse_args();package(a.root,a.output,a.private_key,a.version,a.sequence)
