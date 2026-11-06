#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Fetches the pinned web builds of PdfCraft and GridCraft into web/public/vendor.

They are not built here. Each release file must match its pinned hash, and the
program files stay unchanged: only the large WebAssembly file is stored gzipped,
so the server can send it as it is. See spec 019 and docs/dependencies.md.
"""

import gzip
import hashlib
import io
import pathlib
import shutil
import sys
import urllib.request
import zipfile

REPOSITORY = 'https://github.com/storytold'
ROOT = pathlib.Path(__file__).resolve().parent.parent / 'web' / 'public' / 'vendor'
CHECKSUMS = pathlib.Path(__file__).resolve().parent / 'craft-checksums.sha256'
LEGAL_FILES = ('LICENSE-MIT', 'LICENSE-APACHE', 'NOTICE', 'ATTRIBUTION.md')

BUILDS = (
    {
        'name': 'pdfcraft',
        'version': '0.18.2',
        'sha256': '3f2c11ae9e33d91762eaeb92a0921734a6e6cbab38c8a6a0d715fe1af73c6867',
        'canvas': 'pdfcraft',
    },
    {
        'name': 'gridcraft',
        'version': '0.16.1',
        'sha256': '496785cf8b1f406abeef8531e323961a13170be3815b6ac6809499adea598368',
        'canvas': 'gridcraft_canvas',
    },
)

HOST_PAGE = """<!doctype html>
<html lang="en" data-entry="{entry}" data-canvas="{canvas}">
  <head>
    <meta charset="utf-8" />
    <title>{name}</title>
    <style>
      html, body {{ margin: 0; height: 100%; overflow: hidden; }}
      canvas {{ width: 100%; height: 100%; display: block; outline: none; }}
    </style>
  </head>
  <body>
    <canvas id="{canvas}"></canvas>
    <script type="module" src="../craft-host.js"></script>
  </body>
</html>
"""


def download(url: str) -> bytes:
    """Returns the bytes at a public address."""
    request = urllib.request.Request(url, headers={'User-Agent': 'diffluence-build'})
    with urllib.request.urlopen(request, timeout=300) as response:
        return response.read()


def unpack(build: dict[str, str], archive: bytes) -> None:
    """Puts the program files and the legal files of one build in its folder."""
    target = ROOT / build['name']
    shutil.rmtree(target, ignore_errors=True)
    entry = ''
    with zipfile.ZipFile(io.BytesIO(archive)) as files:
        for item in files.infolist():
            if item.is_dir():
                continue
            name = pathlib.PurePosixPath(item.filename)
            relative = pathlib.PurePosixPath(*name.parts[1:])
            keep = name.suffix in ('.js', '.wasm') or name.name in LEGAL_FILES
            if not keep or '..' in name.parts:
                continue
            path = target / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            data = files.read(item)
            if name.suffix == '.wasm':
                path = path.with_name(path.name + '.gz')
                data = gzip.compress(data, compresslevel=9, mtime=0)
            elif name.suffix == '.js' and len(relative.parts) == 1:
                entry = relative.stem
            path.write_bytes(data)
    page = HOST_PAGE.format(entry=entry, canvas=build['canvas'], name=build['name'])
    (target / 'host.html').write_text(page, encoding='utf-8')


def fetch(build: dict[str, str]) -> None:
    """Downloads one build, and refuses anything but the pinned file."""
    name, version = build['name'], build['version']
    base = f'{REPOSITORY}/{name}'
    archive = download(f'{base}/releases/download/v{version}/{name}-web-{version}.zip')
    digest = hashlib.sha256(archive).hexdigest()
    if digest != build['sha256']:
        sys.exit(f'{name}: the download does not match the pinned hash: {digest}')
    unpack(build, archive)
    attribution = ROOT / name / 'ATTRIBUTION.md'
    if not attribution.exists():
        attribution.write_bytes(download(f'https://raw.githubusercontent.com/storytold/{name}/v{version}/ATTRIBUTION.md'))
    print(name, version, 'ok')


def write_checksums() -> None:
    """Records every stored file, for the test that keeps them unchanged."""
    lines = []
    for build in BUILDS:
        for path in sorted((ROOT / build['name']).rglob('*')):
            if path.is_file():
                lines.append(f'{hashlib.sha256(path.read_bytes()).hexdigest()}  {path.relative_to(ROOT).as_posix()}\n')
    CHECKSUMS.write_text(''.join(lines), encoding='utf-8')


def main() -> None:
    """Fetches every build."""
    for build in BUILDS:
        fetch(build)
    write_checksums()


if __name__ == '__main__':
    main()
