#!/usr/bin/env python3
"""Independent-site regression using the existing r6/r7 routed-netns rig.
Run as root; creates only uks{N} namespaces and a new per-site-* result folder.
"""
import argparse, http.server, json, os, pathlib, re, signal, sqlite3, subprocess, sys, time
from datetime import datetime

def serve(root, port):
    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            bad = self.path == '/parent' or self.path == '/child'
            self.send_response(503 if bad else 200); self.end_headers(); self.wfile.write(b'fixture')
        def do_POST(self):
            body = self.rfile.read(min(int(self.headers.get('Content-Length', 0)), 1 << 20))
            with open(root / 'hooks.jsonl', 'a') as f:
                f.write(json.dumps({'at': time.time(), 'body': json.loads(body)}) + '\n')
            self.send_response(200); self.end_headers()
        def log_message(self, *_): pass
    http.server.ThreadingHTTPServer(('10.77.9.3' if port==19180 else '10.77.9.2', port), Handler).serve_forever()

def main():
    p = argparse.ArgumentParser(); p.add_argument('--nodes', type=int, choices=[2,3,5], required=True)
    p.add_argument('--rig', required=True); p.add_argument('--binary', required=True)
    p.add_argument('--network', default='/workspace/r7/net.sh'); args = p.parse_args()
    assert os.geteuid() == 0, 'run with sudo'
    names = list('abcde'[:args.nodes]); prefix = f'uks{args.nodes}'
    root = pathlib.Path(args.rig) / ('per-site-' + str(args.nodes) + '-' + datetime.now().strftime('%H%M%S'))
    root.mkdir(parents=True); processes = {}; results = []
    network = root / 'network.sh'; network.write_text(pathlib.Path(args.network).read_text().replace('ukm', prefix))
    def run(cmd, **kw): return subprocess.run(cmd, check=True, text=True, capture_output=True, **kw)
    def ip(node, *cmd): return ['ip','netns','exec',prefix+node,*map(str,cmd)]
    def mark(label):
        results.append(label); print(f'PASS {args.nodes}N {label}', flush=True)
        (root/'results.json').write_text(json.dumps({'status':'RUNNING','passed':results}, indent=2))
    def wait(label, check, seconds=65):
        end=time.monotonic()+seconds; last=None
        while time.monotonic()<end:
            try:
                last=check()
                if last: return last
            except (AssertionError,ValueError, KeyError, sqlite3.Error, subprocess.CalledProcessError, FileNotFoundError) as exc: last=str(exc)
            time.sleep(.3)
        raise AssertionError(f'{label}: timed out; last={last}')
    csrf={}
    def api(node, method, path, body=None, allowed=(200,204), timeout=12):
        idx=names.index(node)+1
        cmd=ip(node,'curl','-sS','--max-time',timeout,'-w','\n%{http_code}', '-b',root/f'jar.{node}','-c',root/f'jar.{node}',
               '-X',method,'-H','Content-Type: application/json','-H','X-Unagikuma-CSRF: '+csrf.get(node,''))
        if body is not None: cmd += ['-d',json.dumps(body)]
        cmd += [f'http://10.77.{idx}.2:19110'+path]
        raw=run(cmd).stdout; text,code=raw.rsplit('\n',1);code=int(code)
        if code not in allowed: raise AssertionError(f'{node} {method} {path}: {code} {text}')
        return json.loads(text) if text else None
    def login(node):
        data=api(node,'POST','/api/v1/auth/login',{'username':'admin','password':'Per-site-fixture-9'})
        csrf[node]=data['csrf']
    def start(node):
        idx=names.index(node)+1
        log=open(root/f'log.{node}','a')
        processes[node]=subprocess.Popen(ip(node,args.binary,'server','--data',root/f'd{node}','--listen',f'10.77.{idx}.2:19110'),stdout=log,stderr=log)
    def stop(node):
        proc=processes.get(node)
        if proc and proc.poll() is None:
            proc.terminate()
            try: proc.wait(timeout=12)
            except subprocess.TimeoutExpired: proc.kill();proc.wait()
    def row(node, monitor='web'): return api(node,'GET','/api/v1/monitors/'+monitor)['row']
    def obs(node): return row(node).get('observations',{}).get('rows',[])
    def hooks(node, kind, monitor='web'):
        if not (root/'hooks.jsonl').exists(): return []
        return [v['body'] for line in (root/'hooks.jsonl').read_text().splitlines() if line and
                (v:=json.loads(line))['body'].get('site')==node and v['body'].get('kind')==kind and v['body'].get('monitor')==monitor]
    def target_cut(node,on):
        run(ip(node,'ip','route','add' if on else 'del','blackhole','10.77.9.3/32'))
    def peer_cut(node,on):
        idx=names.index(node)+1
        for other in names:
            if other==node: continue
            oi=names.index(other)+1
            run(ip(node,'ip','route','add' if on else 'del','blackhole',f'10.77.{oi}.2/32'))
            run(ip(other,'ip','route','add' if on else 'del','blackhole',f'10.77.{idx}.2/32'))

    try:
        run(['bash',str(network),'up',' '.join(names)])
        run(ip('t','ip','addr','add','10.77.9.3/24','dev',prefix+'t0'))
        for node in names:
            (root/f'd{node}').mkdir()
            config=f'$FILE revision=2026110601 master={node} interval=1s retry=250ms timeout=400ms attempts=1\n'
            config+='$SET engine.first-round-delay=0s localnet.enabled=off notify.backstop-max=0 cluster.standby-sync=30s cluster.standby-retry=5s cluster.replica-after=10s cluster.node-timeout=10s\n'
            if node=='a':
                config+='hook IN CHANNEL webhook url=http://10.77.9.2:19190/hook\ndefault IN POLICY chain=hook repeat=5s max=3\n'
                config+='web IN HTTP http://10.77.9.3:19180/ok recover-attempts=1\n'
                config+='push-local IN PUSH period=1h nodes=a\n'
            (root/f'd{node}'/'monitors.bind').write_text(config)
        for port in [19180,19190]:
            log=open(root/f'target.{port}.log','w')
            processes[str(port)]=subprocess.Popen(ip('t',sys.executable,__file__,'--serve',root,port),stdout=log,stderr=log)
        wait('target HTTP ready',lambda: run(ip('a','curl','-fsS','--max-time','2','http://10.77.9.3:19180/ok')),20)
        wait('hook HTTP ready',lambda: run(ip('a','curl','-fsS','--max-time','2','http://10.77.9.2:19190/health')),20)
        for node in names: start(node)
        for node in names:
            token=wait('setup token '+node, lambda n=node: (m.group(1) if (m:=re.search(r'setup token: ([0-9a-f]+)',(root/f'log.{n}').read_text())) else None),15)
            data=api(node,'POST','/api/v1/setup',{'username':'admin','password':'Per-site-fixture-9','token':token})
            csrf[node]=data['csrf']
        for node in names[1:]:
            code=api('a','POST','/api/v1/cluster/join-codes',{'server':'https://10.77.1.2:19110'})['code']
            api(node,'POST','/api/v1/cluster/join',{'code':code,'node':node,'title':'site-'+node})
        current=api('a','GET','/api/v1/config')
        api('a','PUT','/api/v1/config',{'text':current['text']+'parent-only IN HTTP http://10.77.9.3:19180/parent nodes=a\nchild IN HTTP http://10.77.9.3:19180/child nodes=b parent=parent-only\n'})
        wait('all sites run web',lambda: all(row(n)['status']=='ok' for n in names),100)
        wait('N independent observations',lambda: len(obs('a'))==args.nodes and all(not x.get('stale') and x['status']=='up' for x in obs('a')),100)
        mark('steady checks + N-source table')
        wait('foreign parent does not suppress child',lambda: row('b','child').get('incident'))
        mark('cross-site parent does not suppress local alert')
        assert api('a','GET','/api/v1/monitors/push-local/push')['token']
        api('b','GET','/api/v1/monitors/push-local/push',allowed=(404,))
        mark('PUSH unique receiving site')
        (root/'hooks.jsonl').write_text('')
        target_cut('b',True)
        problem=wait('b direct alert',lambda: (hooks('b','PROBLEM')+hooks('b','REOPEN')) and (hooks('b','PROBLEM')+hooks('b','REOPEN'))[0])
        assert problem['incident_global_id'].startswith('b:') and len(problem['observations']['rows'])==args.nodes
        wait('table reports isolated failure',lambda: any(x['node_id']=='b' and x['status']=='down' for x in obs('a')))
        assert all(row(n)['status']=='ok' for n in names if n!='b')
        mark('minority failure alerts directly + immutable source snapshot')
        peer_cut('b',True)
        wait('stale peer',lambda: any(x['node_id']=='b' and x.get('stale') for x in obs('a')),25)
        before=row('b')['last_check']
        wait('isolated site keeps checking',lambda: row('b')['last_check']!=before)
        assert row('b')['status']=='critical'
        mark('peer partition: no unknown/voting gate; table stale only')
        peer_cut('b',False)
        wait('fresh peer after heal',lambda: any(x['node_id']=='b' and not x.get('stale') for x in obs('a')))
        target_cut('b',False)
        recovery=wait('source recovery',lambda: hooks('b','RECOVERY') and hooks('b','RECOVERY')[0])
        assert recovery['incident_global_id']==problem['incident_global_id'] and len(recovery['observations']['rows'])==args.nodes
        mark('own recovery keeps source identity and frozen context')
        run(ip('b','ip','route','add','blackhole','10.77.1.2/32'))
        before=row('b')['last_check']
        wait('one-way peer fault does not stop local scheduler',lambda: row('b')['last_check']!=before)
        wait('one-way peer fault marks source stale',lambda: any(x['node_id']=='b' and x.get('stale') for x in obs('a')),25)
        assert row('b')['status']=='ok'
        run(ip('b','ip','route','del','blackhole','10.77.1.2/32'))
        wait('one-way fault heals',lambda: any(x['node_id']=='b' and not x.get('stale') for x in obs('a')))
        mark('one-way routing fault: local state independent and remote stale')
        wait('standby has configuration',lambda: (root/'db'/'replica'/'master.bind').exists(),100)
        stop('a')
        target='c' if args.nodes>2 else 'b';target_cut(target,True)
        wait('direct alert without master',lambda: row(target)['status']=='critical')
        if args.nodes>2: wait('replica to replica delta without master',lambda: any(x['node_id']==target and x['status']=='down' and not x.get('stale') for x in obs('b')))
        assert api('b','GET','/api/v1/config')['mode']=='ro'
        api('b','POST','/api/v1/changes',{'title':'blocked-fixture'},allowed=(409,))
        mark('master outage: independent alerts + peer sync + config read-only')
        target_cut(target,False);start('a')
        wait('old master restart',lambda: api('a','GET','/api/v1/overview'))
        wait('healed sites still check',lambda: all(row(n)['status']=='ok' for n in names),100)
        def imported_child():
            vals=api('a','GET','/api/v1/incidents?state=all&site=b&monitor=child')['items']
            return vals and vals[0]
        imported=wait('archived source incident',imported_child,110)
        assert imported['history_only'] and imported['global_id'].startswith('b:')
        wait('source peer discovered and ACK routed',lambda: api('a','POST',f"/api/v1/incidents/{imported['id']}/ack",{'comment':'ACK routed from a'}) is None,65)
        def source_child(): return next(x for x in api('b','GET','/api/v1/incidents?state=all&monitor=child')['items'] if x['global_id']==imported['global_id'])
        assert source_child()['state']['acked']
        mark('site history filter + source-routed ACK')
        source_before=source_child()['global_id']
        fingerprint=(root/'db'/'site-identity'/'cluster.crt').read_bytes()
        stop('a')
        api('b','POST','/api/v1/cluster/promote',{},allowed=(200,202,204),timeout=60)
        wait('manual promotion restarted',lambda: api('b','GET','/api/v1/config')['mode']=='rw',100)
        assert source_child()['global_id']==source_before and source_child()['state']['acked']
        assert (root/'db'/'site-identity'/'cluster.crt').read_bytes()==fingerprint
        mark('manual promotion preserves incident + ACK + site TLS identity')
        start('a');wait('old master returns as same physical site',lambda: (api('a','GET','/api/v1/cluster/agent').get('member') or {}).get('node')=='a',120)
        wait('post-promotion independent scheduling',lambda: all(row(n)['last_check'] for n in names),100)
        wait('all replicas follow b',lambda: all((api(n,'GET','/api/v1/cluster/agent').get('status') or {}).get('connected') and (api(n,'GET','/api/v1/cluster/agent').get('status') or {}).get('server')=='https://10.77.2.2:19110' for n in names if n!='b'),100)
        wait('post-promotion N-source delta continues',lambda: len(obs('b'))==args.nodes and all(not x.get('stale') and x['status']=='up' for x in obs('b')),100)
        mark('old master rejoins as a; all replicas follow authority; N-source sync continues')
        if args.nodes==2:
            stop('b')
            api('a','POST','/api/v1/cluster/promote',{},allowed=(200,202,204),timeout=60)
            wait('second manual promotion',lambda: api('a','GET','/api/v1/config')['mode']=='rw',100)
            start('b')
            wait('former master with existing NODE rejoins',lambda: (api('b','GET','/api/v1/cluster/agent').get('member') or {}).get('node')=='b',120)
            wait('second authority control connection',lambda: (api('b','GET','/api/v1/cluster/agent').get('status') or {}).get('connected'),100)
            assert source_child()['global_id']==source_before and source_child()['state']['acked']
            wait('second promotion keeps peer observations',lambda: len(obs('a'))==2 and all(not x.get('stale') for x in obs('a')),100)
            mark('second manual promotion preserves existing site name, incident and ACK')
        (root/'results.json').write_text(json.dumps({'status':'PASS','nodes':args.nodes,'passed':results},indent=2))
        print(f'PASS {args.nodes}N ALL; artifacts={root}',flush=True)
    except Exception as exc:
        (root/'results.json').write_text(json.dumps({'status':'FAIL','nodes':args.nodes,'passed':results,'error':str(exc)+str(getattr(exc,'stderr','') or '')},indent=2))
        print(f'FAIL {args.nodes}N {exc}; artifacts={root}',flush=True);raise
    finally:
        for node in list(processes): stop(node)
        subprocess.run(['bash',str(network),'down'],stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)

if __name__=='__main__':
    if len(sys.argv)>1 and sys.argv[1]=='--serve': serve(pathlib.Path(sys.argv[2]),int(sys.argv[3]))
    else: main()
