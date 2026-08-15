#!/usr/bin/env python
"""Transcribe WhatsApp voice notes with faster-whisper. Config lives in whisper_pt.py.

  transcribe.py <chat_jid> <message_id>     one note, downloaded via the bridge
  transcribe.py --latest <chat_jid>         the newest voice note in that chat

  # Batch straight from disk. The bridge names media by second-resolution
  # timestamps and does not re-download an existing file, so asking it for a
  # burst of downloads makes them collide. Reading the store directly avoids
  # that, and the model is loaded once for the whole batch.
  transcribe.py --disk <jid|folder> [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--out file.txt]

Examples:
  transcribe.py --disk 100000000000001@lid --from 2026-06-01
  transcribe.py --disk 100000000000001@lid --from 2026-06-01 --out notes.txt
  transcribe.py --disk "../whatsapp-bridge/store/100000000000001@lid"

Media files are already named after the message timestamp
(audio_YYYYMMDD_HHMMSS.ogg), so date filtering needs no database lookup.
"""
import sys, json, sqlite3, urllib.request, os, glob, re

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import whisper_pt

STORE = os.environ.get("WA_STORE", "../whatsapp-bridge/store")
DB = os.path.join(STORE, "messages.db")
BRIDGE = os.environ.get("WA_BRIDGE", "http://localhost:8080/api/download")


def get_msg(chat_jid, message_id=None, latest=False):
    con = sqlite3.connect(DB)
    con.row_factory = sqlite3.Row
    if latest:
        r = con.execute(
            "SELECT id,chat_jid FROM messages WHERE chat_jid=? AND media_type='audio'"
            " ORDER BY timestamp DESC LIMIT 1", (chat_jid,)).fetchone()
    else:
        r = con.execute("SELECT id,chat_jid FROM messages WHERE id=? AND chat_jid=?",
                        (message_id, chat_jid)).fetchone()
    con.close()
    return (r["id"], r["chat_jid"]) if r else (None, None)


def download(message_id, chat_jid):
    body = json.dumps({"message_id": message_id, "chat_jid": chat_jid}).encode()
    req = urllib.request.Request(BRIDGE, data=body,
                                 headers={"Content-Type": "application/json"})
    resp = json.loads(urllib.request.urlopen(req, timeout=90).read())
    if not resp.get("success"):
        raise RuntimeError(resp.get("message", "download failed"))
    return resp["path"]


def transcribe(path):
    """The model comes from whisper_pt's cache, so a batch loads it once."""
    return whisper_pt.transcribe(path)


def resolve_folder(target):
    """Accept either a folder path or a chat JID under the media store."""
    if os.path.isdir(target):
        return target
    p = os.path.join(STORE, target)
    if os.path.isdir(p):
        return p
    raise SystemExit(f"no folder found for '{target}'")


def file_date(fn):
    """audio_YYYYMMDD_HHMMSS.ogg -> ('2026-06-26', '20:35')"""
    m = re.search(r"(\d{8})_(\d{6})", os.path.basename(fn))
    if not m:
        return None, None
    d, t = m.group(1), m.group(2)
    return f"{d[:4]}-{d[4:6]}-{d[6:]}", f"{t[:2]}:{t[2:4]}"


def batch(target, since=None, until=None, out=None):
    folder = resolve_folder(target)
    files = []
    for p in sorted(glob.glob(os.path.join(folder, "*.ogg"))):
        d, _ = file_date(p)
        if d is None or (since and d < since) or (until and d > until):
            continue
        files.append(p)
    if not files:
        raise SystemExit(f"no audio in {folder}")
    print(f"{len(files)} file(s) in {os.path.basename(folder)}", file=sys.stderr)

    lines = []
    for i, p in enumerate(files, 1):
        d, h = file_date(p)
        print(f"  [{i}/{len(files)}] {d} {h}", file=sys.stderr, flush=True)
        try:
            txt = transcribe(p)
        except Exception as e:
            txt = f"(FAILED: {str(e)[:80]})"
        lines.append(f"{d} {h} | {txt}")

    result = "\n".join(lines)
    if out:
        with open(out, "w", encoding="utf-8") as f:
            f.write(result + "\n")
        print(f"\nsaved to {out}", file=sys.stderr)
    else:
        print(result)


if __name__ == "__main__":
    a = sys.argv[1:]
    if not a:
        print(__doc__)
        sys.exit(1)

    if a[0] == "--disk":
        if len(a) < 2:
            raise SystemExit("--disk needs a jid or folder")

        def opt(name):
            return a[a.index(name) + 1] if name in a else None

        batch(a[1], since=opt("--from"), until=opt("--to"), out=opt("--out"))
        sys.exit(0)

    if a[0] == "--latest":
        mid, cj = get_msg(a[1], latest=True)
    else:
        mid, cj = get_msg(a[0], a[1])
    if not mid:
        print("VOICE NOTE NOT FOUND")
        sys.exit(1)
    print(f"downloading {mid}...", file=sys.stderr)
    path = download(mid, cj)
    print(f"transcribing {os.path.basename(path)}...", file=sys.stderr)
    print("TRANSCRIPT:", transcribe(path))
