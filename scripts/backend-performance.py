#!/usr/bin/env python3
"""Isolated offline A/B resource measurements; Python 3 + psutil required.
Build both sources with identical release flags, or supply prebuilt executables.
Only generated temp data and loopback servers are touched. Never uses user data.
"""
import argparse
import datetime as dt
import hashlib
import http.client
import json
import math
import os
from pathlib import Path
import platform
import re
import secrets
import statistics
import subprocess
import tempfile
import time
import psutil


def summary(values):
    ordered = sorted(values)
    return {"n": len(values), "min": min(values), "median": statistics.median(values),
            "p95": ordered[math.ceil(len(values) * .95) - 1], "max": max(values)}


def footprint(root):
    files = {p.relative_to(root).as_posix(): p.stat().st_size for p in root.rglob('*') if p.is_file()}
    return {"logical_bytes": sum(files.values()), "files": files}


def fixture(groups):
    lines = ['@ IN SOA unagikuma. admin.localhost. ( 2026102301 1m 15s 5s 3 )']
    for group in range(groups):
        members = [f'person{group}_{i}' for i in range(4)]
        lines += [f'{member} IN CHANNEL log' for member in members]
        lines += [f'rotation{group} IN CHANNEL group members={",".join(members)} rotate=daily handoff=09:00 start=2026-01-01 tz=UTC',
                  f'policy{group} IN POLICY chain=rotation{group}']
    return '\n'.join(lines) + '\n'


class Client:
    def __init__(self, port):
        self.conn = http.client.HTTPConnection('127.0.0.1', port, timeout=15)
        self.cookie = ''

    def request(self, path, body=None):
        headers = {'Cookie': self.cookie, 'Content-Type': 'application/json'}
        self.conn.request('GET' if body is None else 'POST', path,
                          None if body is None else json.dumps(body), headers)
        response = self.conn.getresponse()
        data = response.read()
        if response.status != 200:
            raise RuntimeError(f'{path}: HTTP {response.status}: {data[:1000]!r}')
        if response.getheader('Set-Cookie'):
            self.cookie = response.getheader('Set-Cookie').split(';')[0]
        return data


def sample(executable, scenario, args, repetition, scratch):
    with tempfile.TemporaryDirectory(prefix='backend-perf-', dir=scratch) as temporary:
        root = Path(temporary)
        data = root / 'data'
        data.mkdir()
        if scenario == 'representative':
            (data / 'monitors.bind').write_text(fixture(args.groups), encoding='utf-8')
        log_path = root / 'server.log'
        env = {k: v for k, v in os.environ.items() if not k.startswith('UNAGIKUMA_')}
        command = [str(executable), 'server', '--data', str(data), '--listen', '127.0.0.1:0',
                   '--script-dir', str(data / 'check-scripts'), '--instance', 'isolated-performance']
        with log_path.open('wb') as log:
            process = subprocess.Popen(command, cwd=root, env=env, stdout=log, stderr=log)
            client = None
            try:
                deadline = time.monotonic() + 40
                while time.monotonic() < deadline:
                    text = log_path.read_text(encoding='utf-8', errors='replace')
                    match = re.search(r'listen=127\.0\.0\.1:(\d+)', text)
                    if match:
                        client = Client(int(match.group(1)))
                        client.request('/healthz')
                        break
                    if process.poll() is not None:
                        raise RuntimeError(text)
                    time.sleep(.05)
                else:
                    raise RuntimeError('server readiness timeout: ' + text)
                measured = psutil.Process(process.pid)
                time.sleep(args.settle_seconds)
                disk_before = footprint(data)
                cpu_before = measured.cpu_times()
                start = time.perf_counter()
                observations = []
                while True:
                    now = time.perf_counter() - start
                    mem = measured.memory_info()
                    observations.append({'elapsed_s': now, 'rss_bytes': mem.rss,
                                         'private_bytes': getattr(mem, 'private', None),
                                         'threads': measured.num_threads(),
                                         'handles': measured.num_handles() if os.name == 'nt' else measured.num_fds()})
                    if now >= args.idle_seconds:
                        break
                    time.sleep(min(.5, args.idle_seconds - now))
                elapsed = time.perf_counter() - start
                cpu_after = measured.cpu_times()
                cpu = cpu_after.user + cpu_after.system - cpu_before.user - cpu_before.system
                result = {'scenario': scenario, 'repetition': repetition, 'idle_elapsed_s': elapsed,
                          'idle_cpu_s': cpu, 'idle_cpu_percent_one_core': 100 * cpu / elapsed,
                          'rss_bytes': summary([x['rss_bytes'] for x in observations]),
                          'idle_observations': observations, 'disk_before_idle': disk_before,
                          'disk_after_idle': footprint(data)}
                if scenario == 'representative':
                    client.request('/api/v1/setup', {'username': 'benchmark', 'password': secrets.token_urlsafe(32)})
                    paths = ['/api/v1/channels', '/api/v1/policies', '/api/v1/oncall',
                             '/api/v1/oncall/calendar?from=2026-11-01T00:00:00Z&to=2026-11-29T00:00:00Z']
                    api = {}
                    before = measured.memory_info().rss
                    for path in paths:
                        for _ in range(5):
                            client.request(path)
                        timings = []
                        lengths = []
                        last = None
                        cpu0 = measured.cpu_times()
                        for _ in range(args.requests):
                            tick = time.perf_counter_ns()
                            raw = client.request(path)
                            timings.append((time.perf_counter_ns() - tick) / 1e6)
                            lengths.append(len(raw))
                            last = json.loads(raw)
                        cpu1 = measured.cpu_times()
                        assert last is not None
                        expected = args.groups * 5 if path.endswith('/channels') else args.groups
                        count = len(last['rotations']) if '/oncall' in path else len(last)
                        if path.endswith('/policies'):
                            count = sum(1 for item in last if item['name'].startswith('policy'))
                        if count != expected:
                            raise AssertionError(f'{path}: expected {expected} records, got {count}')
                        api[path] = {'milliseconds': summary(timings), 'raw_ms': timings,
                                     'response_bytes': summary(lengths), 'verified_count': count,
                                     'process_cpu_s': cpu1.user + cpu1.system - cpu0.user - cpu0.system}
                    result['api'] = api
                    result['rss_before_api_bytes'] = before
                    result['rss_after_api_bytes'] = measured.memory_info().rss
                    time.sleep(args.settle_seconds)
                    result['rss_after_api_settle_bytes'] = measured.memory_info().rss
                    result['disk_after_api'] = footprint(data)
                result['log_bytes'] = log_path.stat().st_size
                return result
            finally:
                if client:
                    client.conn.close()
                process.terminate()
                process.wait(timeout=15)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--go', default='go')
    parser.add_argument('--baseline-source', type=Path)
    parser.add_argument('--baseline', type=Path, default=Path('bin/perf-baseline.exe'))
    parser.add_argument('--current', type=Path, default=Path('bin/perf-current.exe'))
    parser.add_argument('--output', type=Path, default=Path('test-results/backend-performance.json'))
    parser.add_argument('--repeats', type=int, default=3)
    parser.add_argument('--idle-seconds', type=float, default=10)
    parser.add_argument('--settle-seconds', type=float, default=2)
    parser.add_argument('--requests', type=int, default=50)
    parser.add_argument('--groups', type=int, default=6)
    args = parser.parse_args()
    if min(args.repeats, args.requests, args.groups, args.idle_seconds, args.settle_seconds) <= 0:
        parser.error('sample counts and observation intervals must be positive')
    repo = Path(__file__).resolve().parents[1]
    os.chdir(repo)
    args.output = args.output.resolve()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    scratch = Path(os.environ.get('TMPDIR', str(args.output.parent / 'scratch')))
    scratch.mkdir(parents=True, exist_ok=True)
    # Resolve an already-installed auto-selected toolchain before disabling network.
    goroot = subprocess.check_output([args.go, 'env', 'GOROOT'], text=True).strip()
    args.go = str(Path(goroot) / 'bin' / ('go.exe' if os.name == 'nt' else 'go'))
    build_flags = ['build', '-trimpath', '-buildvcs=false', '-ldflags', '-s -w -X main.version=performance']
    builds = {}
    for label, executable in [('baseline', args.baseline), ('current', args.current)]:
        executable = executable.resolve()
        if args.baseline_source:
            source = args.baseline_source.resolve() if label == 'baseline' else repo
            executable.parent.mkdir(parents=True, exist_ok=True)
            command = [args.go, *build_flags, '-o', str(executable), './cmd/unagikuma']
            subprocess.run(command, cwd=source, env={**os.environ, 'CGO_ENABLED': '0', 'GOPROXY': 'off', 'GOSUMDB': 'off'}, check=True)
        builds[label] = {'executable': str(executable), 'bytes': executable.stat().st_size,
                         'sha256': hashlib.sha256(executable.read_bytes()).hexdigest(),
                         'build_info': subprocess.check_output([args.go, 'version', '-m', str(executable)], text=True)}
    output = {'schema_version': 1, 'started_utc': dt.datetime.now(dt.timezone.utc).isoformat(),
              'environment': {'platform': platform.platform(), 'python': platform.python_version(),
                              'psutil': psutil.__version__, 'logical_cpus': psutil.cpu_count(),
                              'physical_cpus': psutil.cpu_count(logical=False),
                              'total_memory_bytes': psutil.virtual_memory().total,
                              'go': subprocess.check_output([args.go, 'version'], text=True).strip()},
              'method': {'build_flags': build_flags, 'cgo_enabled': 0, 'repeats': args.repeats,
                         'idle_seconds': args.idle_seconds, 'settle_seconds': args.settle_seconds,
                         'requests_per_endpoint_per_run': args.requests, 'warmup_requests': 5,
                         'http': 'loopback HTTP/1.1 persistent connection, sequential; includes client and transport',
                         'disk': 'live logical file sizes, includes SQLite WAL/SHM; not allocated filesystem blocks',
                         'termination': 'TerminateProcess on Windows; live disk measured before forced stop',
                         'fixture': fixture(args.groups), 'monitors': 0, 'incidents': 0,
                         'external_providers': 0, 'order': 'alternating baseline/current each repetition'},
              'git_head': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
              'git_diff_stat': subprocess.check_output(['git', 'diff', '--stat'], text=True),
              'builds': builds, 'samples': []}
    for repetition in range(1, args.repeats + 1):
        labels = ['baseline', 'current'] if repetition % 2 else ['current', 'baseline']
        for scenario in ['empty', 'representative']:
            for label in labels:
                result = sample(Path(builds[label]['executable']), scenario, args, repetition, scratch)
                result['build'] = label
                output['samples'].append(result)
                args.output.write_text(json.dumps(output, indent=2), encoding='utf-8')
                print(f'{label} {scenario} run={repetition} RSS={result["rss_bytes"]["median"]} CPU={result["idle_cpu_s"]:.6f}s disk={result["disk_after_idle"]["logical_bytes"]}', flush=True)
    assert len(output['samples']) == args.repeats * 4
    print(f'Wrote {args.output}', flush=True)


if __name__ == '__main__':
    main()
