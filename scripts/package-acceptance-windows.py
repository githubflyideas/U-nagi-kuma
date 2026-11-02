"""Build dist.sh-equivalent Windows/amd64 candidate and test its extracted executable.
Run from Git Bash with go on PATH: python scripts/package-acceptance-windows.py --output <durable-dir>
Never publishes, installs services, changes production data, or builds other platforms.
"""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import zipfile

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument('--output', required=True, type=Path)
parser.add_argument('--version', default='v1.5.7-rc.1')
args = parser.parse_args()
repo = Path(__file__).resolve().parents[1]
out = args.output.resolve()
out.mkdir(parents=True, exist_ok=True)
if sys.platform != 'win32':
    raise SystemExit('This acceptance runner requires Windows')
if not args.version.startswith('v') or any(c not in 'abcdefghijklmnopqrstuvwxyz0123456789.-' for c in args.version):
    raise SystemExit('Invalid version')
env = dict(os.environ, CGO_ENABLED='0', GOOS='windows', GOARCH='amd64', GOARM='7')
commands = []

def run(argv, log, *, check=True, environment=None):
    with (out / log).open('wb') as stream:
        p = subprocess.run(argv, cwd=repo, env=environment or env, stdout=stream, stderr=subprocess.STDOUT)
    commands.append({'argv': [str(a) for a in argv], 'exit_code': p.returncode, 'log': str(out / log)})
    if check and p.returncode:
        raise RuntimeError(f'{argv} failed with {p.returncode}; see {out / log}')
    return p.returncode

def sha(path):
    return hashlib.file_digest(path.open('rb'), 'sha256').hexdigest()

manifest = {'candidate': args.version, 'platform': 'windows/amd64', 'status': 'building',
            'started_utc': datetime.now(timezone.utc).isoformat(), 'commands': commands,
            'report': str(out / 'report.json'), 'free_bytes_before': shutil.disk_usage(out).free}
coordination = repo / 'docs/package-acceptance-candidate.json'

def save():
    text = json.dumps(manifest, indent=2) + '\n'
    (out / 'report.json').write_text(text, encoding='utf-8')
    coordination.write_text(text, encoding='utf-8')

try:
    if manifest['free_bytes_before'] < 2 * 1024**3:
        raise RuntimeError('Less than 2 GiB free')
    manifest['revision'] = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=repo, text=True).strip()
    run(['git', 'status', '--short'], 'git-status.log')
    run(['go', 'version'], 'go-version.log')
    name = f'unagikuma-{args.version}-windows-amd64'
    archive = out / (name + '.zip')
    if archive.exists():
        raise RuntimeError('Refusing to overwrite existing archive; choose a fresh output directory')
    stage = Path(tempfile.mkdtemp(prefix='stage-', dir=out)) / name
    stage.mkdir()
    run(['bash', 'scripts/licenses.sh'], 'licenses-generation.log')
    shutil.copyfile(out / 'licenses-generation.log', stage / 'THIRD_PARTY_LICENSES.txt')
    run(['go', 'build', '-trimpath', '-ldflags', f'-s -w -X main.version={args.version}',
         '-o', str(stage / 'unagikuma.exe'), './cmd/unagikuma'], 'build.log')
    for source in [repo / 'LICENSE', *sorted(repo.glob('README*.md')),
                   repo / 'docs/manual.md', repo / 'docs/manual.zh-CN.md', repo / 'docs/spec/monitors-bind.md']:
        dest = stage / source.relative_to(repo)
        dest.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, dest)
    with zipfile.ZipFile(archive, 'w', compression=zipfile.ZIP_DEFLATED) as z:
        for file in sorted(stage.rglob('*')):
            if file.is_file():
                z.write(file, file.relative_to(stage.parent).as_posix())
    # A fresh, retained scratch extraction, not a stage binary or system temp.
    extracted = Path(tempfile.mkdtemp(prefix='extracted-', dir=out))
    with zipfile.ZipFile(archive) as z:
        assert z.testzip() is None
        z.extractall(extracted)
    binary = extracted / name / 'unagikuma.exe'
    entries = {str(p.relative_to(stage)): sha(p) for p in stage.rglob('*') if p.is_file()}
    actual = {str(p.relative_to(extracted / name)): sha(p) for p in (extracted / name).rglob('*') if p.is_file()}
    if actual != entries:
        raise RuntimeError('Extracted contents differ from packaged stage')
    manifest.update(archive=str(archive), archive_sha256=sha(archive), extracted_binary=str(binary),
                    binary_sha256=sha(binary), archive_contents=entries, status='built')
    (out / 'SHA256SUMS').write_text(f"{sha(archive)}  {archive.name}\n{sha(binary)}  {binary}\n", encoding='utf-8')
    save()
    run([str(binary), 'version'], 'binary-version.log')
    if (out / 'binary-version.log').read_text().strip() != f'unagikuma {args.version}':
        raise RuntimeError('Candidate version mismatch')
    run(['go', 'version', '-m', str(binary)], 'binary-build-info.log')
    testexe = out / 'e2e.test.exe'
    run(['go', 'test', '-tags', 'e2e', '-c', '-o', str(testexe), './e2e'], 'compile-harness.log')
    run([sys.executable, 'scripts/package-acceptance-harness-test.py', str(testexe)], 'harness-green.log')
    screenshots = out / 'screenshots'
    screenshots.mkdir()
    testenv = dict(env, UNAGIKUMA_E2E_BIN=str(binary), UNAGIKUMA_E2E_UI='1', UNAGIKUMA_E2E_OUT=str(screenshots))
    code = run(['go', 'test', '-tags', 'e2e', '-count=1', '-timeout=25m', '-json', './e2e'],
               'e2e.jsonl', check=False, environment=testenv)
    events = []
    for line in (out / 'e2e.jsonl').read_text(encoding='utf-8').splitlines():
        if line.startswith('{'):
            events.append(json.loads(line))
    (out / 'e2e.log').write_text(''.join(e.get('Output', '') for e in events), encoding='utf-8')
    external = {'TestFailover', 'TestStandbyWindowsSupervisedRecovery', 'TestUI', 'TestZonesUI', 'TestImportAdaptersUI'}
    results = [e for e in events if e.get('Test') and e['Action'] in ('pass', 'fail', 'skip')]
    for e in results:
        e['coverage_kind'] = 'extracted-executable' if e['Test'] in external else ('browser-fixture' if e['Test'].startswith('TestUIComponents') else 'source-in-process')
    manifest['tests'] = results
    manifest['top_level_counts'] = {a: sum(e['Action'] == a and '/' not in e['Test'] for e in results) for a in ('pass', 'fail', 'skip')}
    manifest['external_tests_expected'] = sorted(external)
    manifest['binary_unchanged_after_tests'] = sha(binary) == manifest['binary_sha256']
    manifest['archive_unchanged_after_tests'] = sha(archive) == manifest['archive_sha256']
    manifest['limitations'] = ['Windows amd64 only; no other platform acceptance', 'Not an official tagged/published release',
        'Windows supervisor test is NOT SCM install/recovery acceptance', 'UI components use mocked API fixtures; runtime engine tests execute source in process',
        'Packaged README images are existing repository documentation; final candidate capture is separate']
    passed = {e['Test'] for e in results if e['Action'] == 'pass'}
    if code or not external.issubset(passed) or any(e['Action'] in ('fail', 'skip') for e in results):
        raise RuntimeError('Acceptance has failed/missing/skipped tests; inspect e2e.log')
    if not manifest['binary_unchanged_after_tests'] or not manifest['archive_unchanged_after_tests']:
        raise RuntimeError('Artifact hash changed during acceptance')
    manifest['status'] = 'passed-windows-amd64-scoped-acceptance'
except Exception as exc:
    manifest.update(status='failed', error=str(exc))
    raise
finally:
    manifest['finished_utc'] = datetime.now(timezone.utc).isoformat()
    save()
    print(json.dumps(manifest, indent=2))
