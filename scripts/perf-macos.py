#!/usr/bin/env python3
"""Disposable fixed fixtures and native macOS footprint sampling (no installs).

python3 scripts/perf-macos.py fixtures /tmp/yohaku-perf
python3 scripts/perf-macos.py measure /tmp/yohaku-perf --app build/Yohaku.app \
    --kind yohaku --output /tmp/yohaku-before.json

Heynote uses its documented HEYNOTE_TEST_USER_DATA_DIR. Neither application
touches the real user library. AX window availability is not input-to-screen
latency or first-edit readiness. Summed process footprints include shared costs
and are not a system-wide unique-memory measurement. Run apps sequentially.
"""

import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import platform
import random
import re
import shutil
import statistics
import struct
import subprocess
import tempfile
import time
import zlib


def write_json(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2))


def png_image(width, height, noisy=False):
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

    rng = random.Random(0)
    rows = b"".join(b"\0" + (rng.randbytes(width * 3) if noisy else bytes([60 + y % 100, 110, 170]) * width)
                    for y in range(height))
    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(rows)) + chunk(b"IEND", b""))


def fixtures(root):
    # Deliberately not random: exact prose/blocks are shared by the two formats.
    text = "\n".join(f"{i:04d} 留一点余白 — a thought to keep, not another feature." for i in range(1500))
    code = "\n".join(f'const thought{i} = {{ title: "余白", done: false }};' for i in range(240))
    scenarios = {
        "blank": [[("Plain Text", "text", "")]],
        "text10": [[("Plain Text", "text", text)] for _ in range(10)],
        "text100": [[("Plain Text", "text", text)] for _ in range(100)],
        "code30": [[("JavaScript", "javascript", code), ("Python", "python", 'print("余白")\n' * 80)] for _ in range(30)],
        "math": [[("Math", "math", "price = 19\nquantity = 3\nprice * quantity\nprev + 5\n13 inches in cm\nsqrt(81)")]],
        "images": [[("Plain Text", "text", "Image library control")]],
        "image-heavy": [[("Plain Text", "text", "Image library control")]],
    }
    manifest = {}
    for name, documents in scenarios.items():
        target = root / name
        notes, hashes = [], []
        for i, blocks in enumerate(documents):
            title = f"Note {i + 1:02d} · {name}"
            body = "\n".join(f"⟦yohaku:{language}⟧\n{body}" for language, _, body in blocks)
            notes.append({"id": f"perf-{name}-{i}", "title": title, "text": body,
                          "selections": [{"Anchor": len(f"⟦yohaku:{blocks[0][0]}⟧\n".encode()),
                                          "Head": len(f"⟦yohaku:{blocks[0][0]}⟧\n".encode())}], "scroll": 0})
            source = json.dumps({"formatVersion": "2.0.0", "name": title}, ensure_ascii=False) + "\n"
            source += "\n".join(f"∞∞∞{token}\n{body}" for _, token, body in blocks)
            filename = "scratch.txt" if i == 0 else f"note-{i:02d}.txt"
            path = target / "heynote" / "notes" / filename
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(source)
            hashes.append([hashlib.sha256(body.encode()).hexdigest() for _, _, body in blocks])
        closed = []
        if name in ("images", "image-heavy"):
            url = "data:image/png;base64," + base64.b64encode(png_image(1200, 800, noisy=name == "image-heavy")).decode()
            for i in range(8):
                rich = {"id": f"perf-image-{i}", "title": f"Image {i + 1}",
                        "text": "⟦yohaku:Plain Text⟧\n", "scroll": 0,
                        "rich": {"text": {"paragraphs": [
                            {"style": {}, "runs": [{"text": "before 中文", "attributes": {}}]},
                            {"style": {}, "runs": [{"text": "\ufffc", "attributes": {}, "attachment": {
                                "url": url, "name": f"Fixed image {i}", "width": 480, "height": 320}}]},
                            {"style": {}, "runs": [{"text": "after 图片", "attributes": {}}]}]},
                            "anchor": 0, "caret": 0, "scroll": 0}}
                (notes if i < 4 else closed).append(rich)
        write_json(target / "yohaku" / "session.json", {"version": 2 if name in ("images", "image-heavy") else 1,
                   "open": notes, "closed": closed, "active": 0})
        write_json(target / "heynote" / "config.json", {
            "settings": {"autoUpdate": False, "autoInstallUpdates": False, "showLeftPanel": False},
            "theme": "light", "windowConfig": {"width": 1080, "height": 800, "visibleOnQuit": True},
            "openTabsState": {"currentBufferPath": "scratch.txt", "openTabs": [
                "scratch.txt" if i == 0 else f"note-{i:02d}.txt" for i in range(len(documents))], "recentBuffers": []}})
        manifest[name] = {"documents": len(documents), "body_sha256": hashes,
                          "body_utf8_bytes": sum(len(b.encode()) for doc in documents for _, _, b in doc),
                          "images": "8 × 1200×800, identical PNG, 4 open + 4 archived, text active" if name in ("images", "image-heavy") else None}
    write_json(root / "manifest.json", manifest)


def footprint(pid):
    result = subprocess.run(["vmmap", "-summary", str(pid)], capture_output=True, text=True, check=True)
    match = re.search(r"^Physical footprint:\s+([\d.]+)([KMGT]?)", result.stdout, re.M)
    peak = re.search(r"^Physical footprint \(peak\):\s+([\d.]+)([KMGT]?)", result.stdout, re.M)
    if not match or not peak:
        raise RuntimeError("vmmap footprint missing")
    return [float(m[1]) * 1024 ** (" KMGT".index(m[2]) if m[2] else 0) for m in [match, peak]]


def processes(root_pid, app):
    lines = subprocess.check_output(["ps", "-axo", "pid=,ppid=,command="], text=True).splitlines()
    rows = [line.strip().split(None, 2) for line in lines if line.strip()]
    found = {root_pid}
    for _ in range(5):
        found.update(int(pid) for pid, parent, command in rows if int(parent) in found or str(app / "Contents") in command)
    return sorted(found)


def measure(args):
    if platform.system() != "Darwin":
        raise SystemExit("Native measurement requires macOS")
    app = args.app.resolve()
    executable = app / "Contents" / "MacOS" / ("Yohaku" if args.kind == "yohaku" else "Heynote")
    results = {"platform": platform.platform(), "app": str(app), "kind": args.kind,
               "idle_seconds": args.idle, "runs": [], "metric": "vmmap physical footprint, sum of app and associated processes"}
    for scenario in args.scenarios:
        if scenario in ("images", "image-heavy") and args.kind != "yohaku":
            continue
        for repeat in range(3):
            with tempfile.TemporaryDirectory(prefix="yohaku-perf-run-") as tmp:
                data = Path(tmp) / "data"
                shutil.copytree(args.root / scenario / args.kind, data)
                env = dict(os.environ)
                command = [str(executable)]
                if args.kind == "yohaku":
                    command += ["-data-dir", str(data)]
                else:
                    env["HEYNOTE_TEST_USER_DATA_DIR"] = str(data)
                with (Path(tmp) / "stderr.log").open("w") as log:
                    started = time.monotonic()
                    proc = subprocess.Popen(command, env=env, stdout=log, stderr=log)
                    pids = [proc.pid]
                    try:
                        window_ms = None
                        while time.monotonic() - started < 20:
                            if proc.poll() is not None:
                                raise RuntimeError((Path(tmp) / "stderr.log").read_text())
                            check = subprocess.run(["osascript", "-e", 'tell application "System Events" to count windows of '
                                                    f"(first process whose unix id is {proc.pid})"], capture_output=True, text=True)
                            if check.returncode == 0 and check.stdout.strip().isdigit() and int(check.stdout) > 0:
                                window_ms = (time.monotonic() - started) * 1000
                                break
                            time.sleep(0.05)
                        if window_ms is None:
                            raise RuntimeError("No accessible native window within 20s")
                        if args.kind == "yohaku":
                            title = subprocess.check_output(["osascript", "-e", 'tell application "System Events" to get name of window 1 of '
                                                             f"(first process whose unix id is {proc.pid})"], text=True).strip()
                            if title != "Yohaku · 余白":
                                raise RuntimeError(f"Not an editor window: {title!r}; " + (Path(tmp) / "stderr.log").read_text())
                        time.sleep(max(0, args.idle - (time.monotonic() - started)))
                        pids = processes(proc.pid, app)
                        samples = []
                        for _ in range(3):
                            sample = [{"pid": pid, "footprint_bytes": fp, "peak_bytes": peak}
                                      for pid in pids for fp, peak in [footprint(pid)]]
                            samples.append({"total_bytes": sum(s["footprint_bytes"] for s in sample), "processes": sample})
                            time.sleep(0.4)
                        row = {"scenario": scenario, "run": repeat + 1, "ax_window_ms": window_ms,
                               "idle_bytes": statistics.median(s["total_bytes"] for s in samples), "samples": samples}
                        results["runs"].append(row)
                        write_json(args.output, results)
                        print(f'{scenario} #{repeat + 1}: {row["idle_bytes"] / 1024**2:.1f} MiB, AX window {window_ms:.0f} ms, {len(pids)} process(es)', flush=True)
                    finally:
                        # All processes/data here were created for this isolated run.
                        for pid in reversed(pids):
                            try:
                                os.kill(pid, 15)
                            except ProcessLookupError:
                                pass
                        try:
                            proc.wait(timeout=10)
                        except subprocess.TimeoutExpired:
                            proc.kill()
                            proc.wait()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    sub = parser.add_subparsers(dest="operation", required=True)
    fixture_parser = sub.add_parser("fixtures")
    fixture_parser.add_argument("root", type=Path)
    measurement = sub.add_parser("measure")
    measurement.add_argument("root", type=Path)
    measurement.add_argument("--app", type=Path, required=True)
    measurement.add_argument("--kind", choices=["yohaku", "heynote"], required=True)
    measurement.add_argument("--output", type=Path, required=True)
    measurement.add_argument("--idle", type=float, default=6)
    measurement.add_argument("--scenarios", nargs="+", default=["blank", "text10", "code30", "math", "images"])
    options = parser.parse_args()
    if options.operation == "fixtures":
        fixtures(options.root)
    else:
        measure(options)
