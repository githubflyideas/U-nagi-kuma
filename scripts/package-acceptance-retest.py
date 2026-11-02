"""Retest a retained extracted package without rebuilding or replacing initial evidence."""
import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys

p = argparse.ArgumentParser(description=__doc__)
p.add_argument('--report', required=True, type=Path)
p.add_argument('--label', default='retest')
a = p.parse_args()
if not a.label or any(c not in 'abcdefghijklmnopqrstuvwxyz0123456789-' for c in a.label):
    raise SystemExit('Invalid retest label')
repo = Path(__file__).resolve().parents[1]
initial = json.loads(a.report.read_text(encoding='utf-8'))
out = a.report.resolve().parent / a.label
out.mkdir(exist_ok=False)
binary = Path(initial['extracted_binary'])
archive = Path(initial['archive'])
def sha(path):
    with path.open('rb') as f:
        return hashlib.file_digest(f, 'sha256').hexdigest()
for path, key in ((binary, 'binary_sha256'), (archive, 'archive_sha256')):
    if sha(path) != initial[key]:
        raise SystemExit(f'Artifact hash differs: {path}')
report = dict(initial)
report.update(status='retesting', initial_report=str(a.report.resolve()), report=str(out / 'report.json'),
              started_utc=datetime.now(timezone.utc).isoformat(), commands=[], tests=[],
              harness_revision_note='Windows TestFailover explicitly supervises exit 3; retirement wait spans the documented one-minute retry interval; product binary unchanged')
report.pop('error', None)
env = dict(os.environ, UNAGIKUMA_E2E_BIN=str(binary), UNAGIKUMA_E2E_UI='1', UNAGIKUMA_E2E_OUT=str(out / 'screenshots'))
(out / 'screenshots').mkdir()
def run(argv, log):
    with (out / log).open('wb') as f:
        result = subprocess.run(argv, cwd=repo, env=env, stdout=f, stderr=subprocess.STDOUT)
    report['commands'].append({'argv': [str(x) for x in argv], 'exit_code': result.returncode, 'log': str(out / log)})
    return result.returncode
try:
    testexe = out / 'e2e.test.exe'
    if run(['go', 'test', '-tags', 'e2e', '-c', '-o', str(testexe), './e2e'], 'compile-harness.log'):
        raise RuntimeError('Harness compilation failed')
    if run([sys.executable, 'scripts/package-acceptance-harness-test.py', str(testexe)], 'harness-green.log'):
        raise RuntimeError('Harness validation failed')
    code = run(['go', 'test', '-tags', 'e2e', '-count=1', '-timeout=25m', '-json', './e2e'], 'e2e.jsonl')
    events = [json.loads(line) for line in (out / 'e2e.jsonl').read_text(encoding='utf-8').splitlines() if line.startswith('{')]
    (out / 'e2e.log').write_text(''.join(e.get('Output', '') for e in events), encoding='utf-8')
    results = [e for e in events if e.get('Test') and e['Action'] in ('pass', 'fail', 'skip')]
    external = set(initial['external_tests_expected'])
    for e in results:
        e['coverage_kind'] = 'extracted-executable' if e['Test'] in external else ('browser-fixture' if e['Test'].startswith('TestUIComponents') else 'source-in-process')
    report['tests'] = results
    report['top_level_counts'] = {action: sum(e['Action'] == action and '/' not in e['Test'] for e in results) for action in ('pass', 'fail', 'skip')}
    report['archive_unchanged_after_tests'] = sha(archive) == initial['archive_sha256']
    report['binary_unchanged_after_tests'] = sha(binary) == initial['binary_sha256']
    passed = {e['Test'] for e in results if e['Action'] == 'pass'}
    if code or not external.issubset(passed) or any(e['Action'] != 'pass' for e in results):
        raise RuntimeError('Failed, missing or skipped tests; inspect e2e.log')
    if not report['archive_unchanged_after_tests'] or not report['binary_unchanged_after_tests']:
        raise RuntimeError('Artifact changed during retest')
    report['status'] = 'passed-windows-amd64-scoped-acceptance'
except Exception as exc:
    report.update(status='failed', error=str(exc))
    raise
finally:
    report['finished_utc'] = datetime.now(timezone.utc).isoformat()
    text = json.dumps(report, indent=2) + '\n'
    (out / 'report.json').write_text(text, encoding='utf-8')
    (repo / 'docs/package-acceptance-candidate.json').write_text(text, encoding='utf-8')
    print(text)
