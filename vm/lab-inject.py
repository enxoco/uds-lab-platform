#!/usr/bin/env python3
import http.server
import subprocess
import json
import os
import socket
import struct
import urllib.request
from urllib.parse import urlparse, parse_qs


def cdp_navigate(url):
    targets = json.loads(urllib.request.urlopen('http://127.0.0.1:9222/json').read())
    pages = [t for t in targets if t.get('type') == 'page']
    if not pages:
        return
    ws_url = pages[0]['webSocketDebuggerUrl'].replace('ws://', '')
    host_port, path = ws_url.split('/', 1)
    path = '/' + path
    sock = socket.create_connection(('127.0.0.1', 9222), timeout=3)
    key = __import__('base64').b64encode(os.urandom(16)).decode()
    sock.sendall((
        f'GET {path} HTTP/1.1\r\nHost: 127.0.0.1:9222\r\n'
        f'Upgrade: websocket\r\nConnection: Upgrade\r\n'
        f'Sec-WebSocket-Key: {key}\r\nSec-WebSocket-Version: 13\r\n\r\n'
    ).encode())
    buf = b''
    while b'\r\n\r\n' not in buf:
        buf += sock.recv(4096)
    msg = json.dumps({'id': 1, 'method': 'Page.navigate', 'params': {'url': url}}).encode()
    mask = os.urandom(4)
    masked = bytes([b ^ mask[i % 4] for i, b in enumerate(msg)])
    n = len(msg)
    header = bytes([0x81, 0x80 | n]) if n < 126 else bytes([0x81, 0xFE]) + struct.pack('>H', n)
    sock.sendall(header + mask + masked)
    sock.close()


class Handler(http.server.BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get('Content-Length', 0))
        try:
            body = json.loads(self.rfile.read(length))
        except Exception:
            body = {}

        if self.path == '/cmd':
            try:
                data = body.get('data', '')
                if data:
                    subprocess.run(['tmux', 'send-keys', '-t', 'main', '-l', data])
                    subprocess.run(['tmux', 'send-keys', '-t', 'main', 'Enter'])
            except Exception:
                pass
            self.send_response(200)
            self.end_headers()

        elif self.path == '/verify':
            step = body.get('step', '')
            script = f'/opt/scenario/verify/step{step}.sh'
            passed = False
            try:
                if os.path.exists(script):
                    result = subprocess.run(
                        ['bash', script],
                        capture_output=True,
                        timeout=30,
                    )
                    passed = result.returncode == 0
            except Exception:
                pass
            response = json.dumps({'pass': passed}).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(response)))
            self.end_headers()
            self.wfile.write(response)

        elif self.path == '/navigate':
            url = body.get('url', '')
            if url:
                try:
                    cdp_navigate(url)
                except Exception:
                    pass
            self.send_response(200)
            self.end_headers()

        else:
            self.send_response(404)
            self.end_headers()

    def do_GET(self):
        parsed = urlparse(self.path)
        params = parse_qs(parsed.query)

        if parsed.path == '/services':
            try:
                result = subprocess.run(
                    ['uds', 'zarf', 'tools', 'kubectl', 'get',
                     'virtualservices', '-A', '-o', 'json'],
                    capture_output=True, timeout=10,
                    env={**os.environ, 'KUBECONFIG': '/root/.kube/config', 'HOME': '/root'},
                )
                data = json.loads(result.stdout)
                services = []
                seen = set()
                for item in data.get('items', []):
                    name = item['metadata']['name']
                    for host in item.get('spec', {}).get('hosts', []):
                        if host.endswith('.uds.dev') and host not in seen:
                            seen.add(host)
                            services.append({'label': name, 'url': 'https://' + host})
                response = json.dumps(services).encode()
            except Exception:
                response = b'[]'
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(response)))
            self.end_headers()
            self.wfile.write(response)

        elif parsed.path == '/files':
            path = params.get('path', ['/root'])[0]
            path = os.path.realpath(path)
            if not path.startswith('/root'):
                self.send_response(403); self.end_headers(); return
            try:
                if os.path.isdir(path):
                    entries = sorted(
                        [{'name': n, 'path': os.path.join(path, n),
                          'type': 'dir' if os.path.isdir(os.path.join(path, n)) else 'file'}
                         for n in os.listdir(path)],
                        key=lambda e: (e['type'] == 'file', e['name'].lower())
                    )
                    response = json.dumps(entries).encode()
                    ct = 'application/json'
                elif os.path.isfile(path):
                    with open(path, 'rb') as f:
                        response = f.read()
                    ct = 'text/plain; charset=utf-8'
                else:
                    self.send_response(404); self.end_headers(); return
                self.send_response(200)
                self.send_header('Content-Type', ct)
                self.send_header('Content-Length', str(len(response)))
                self.end_headers()
                self.wfile.write(response)
            except Exception as e:
                self.send_response(500); self.end_headers()

        else:
            self.send_response(404)
            self.end_headers()

    def do_PUT(self):
        parsed = urlparse(self.path)
        params = parse_qs(parsed.query)
        length = int(self.headers.get('Content-Length', 0))

        if parsed.path == '/files':
            path = params.get('path', [''])[0]
            path = os.path.realpath(path)
            if not path or not path.startswith('/root'):
                self.send_response(403); self.end_headers(); return
            try:
                content = self.rfile.read(length)
                os.makedirs(os.path.dirname(path), exist_ok=True)
                with open(path, 'wb') as f:
                    f.write(content)
                self.send_response(200)
            except Exception:
                self.send_response(500)
            self.end_headers()
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, *args): pass


http.server.HTTPServer(('0.0.0.0', 7680), Handler).serve_forever()
