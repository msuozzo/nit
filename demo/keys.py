# /// script
# dependencies = ["pillow"]
# ///
"""Records demo/pi.mp4 with demo/pi.tape, then draws the keys the tape sends
onto it, each with what it does, as demo/pi-keys.mp4. Run from the repository
root, on macOS for its SF fonts:

  uv run demo/keys.py

Text typed into pi's prompt or a nit comment isn't drawn, since it shows on
screen as it's typed.
"""
import subprocess
import tempfile
from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter, ImageFont

DEMO = Path(__file__).parent
W, H, SS = 1280, 720, 2  # pi.tape's size, and supersampling for smooth edges
NIT = 0.6  # the share of the width nit's pane takes, as pi-setup.sh's tmux.conf sets
TYPING = 0.060  # pi.tape's TypingSpeed
HOLD, FADE_IN, FADE_OUT = 1.2, 0.10, 0.20

SF = "/System/Library/Fonts/SFNS.ttf"
MONO = "/System/Library/Fonts/SFNSMono.ttf"


def record():
    """Runs pi.tape, returning each line VHS prints as (frames so far, line).

    VHS captures fewer frames than it plays back when it can't keep up, so the
    video runs shorter than the recording did. A command's place in the video
    is the frames captured before it, counted where VHS keeps them.
    """
    tmp = Path(tempfile.gettempdir())
    old = set(tmp.glob("vhs*"))
    lines = []
    with subprocess.Popen(["vhs", str(DEMO / "pi.tape")], stdout=subprocess.PIPE, text=True) as vhs:
        for line in vhs.stdout:
            print(line, end="")
            frames = sum(1 for d in set(tmp.glob("vhs*")) - old for _ in d.glob("frame-text-*.png"))
            lines.append((frames, line.strip()))
    if vhs.returncode:
        raise SystemExit(vhs.returncode)
    return lines


def commands(lines, dur):
    """Returns the tape's commands between Show and Hide, as (video time, command)."""
    show, out = None, []
    for n, cmd in lines:
        if show is None:
            show = n if cmd == "Show" else None
        elif cmd == "Hide":
            return [(dur * (m - show) / (n - show), c) for m, c in out]
        else:
            out.append((n, cmd))
    raise SystemExit("no Show and Hide in pi.tape's output")


def groups(cmds):
    """Turns commands into keys to draw: (caption, center x, [(time, keys so far)]).

    Keys go to pi until nit opens in its pane on the right, and to nit until it closes.
    """
    out, mode, nit, search = [], "prompt", False, None
    for t, cmd in cmds:
        name, _, arg = cmd.partition(" ")
        cx = W - int(W * NIT) // 2 if nit else W // 2
        if name == "Wait" and arg == "Screen NORMAL":
            nit = True
        elif name == "Wait" and "{100}" in arg:
            nit = False
        elif name == "Enter" and mode == "prompt":
            out.append(("send", cx, [(t, [("key", "⏎")])]))
            mode = "nit"
        elif name == "Enter" and mode == "search":
            search.append((t, search[-1][1] + [("key", "⏎")]))
            mode = "nit"
        elif name == "Type" and mode == "nit" and arg.startswith("/"):
            search = [(t + k * TYPING, [("key", "/")] + ([("text", arg[1:k + 1])] if k else [])) for k in range(len(arg))]
            out.append(("search", cx, search))
            mode = "search"
        elif name == "Type" and mode == "nit" and arg == "c":
            out.append(("comment", cx, [(t, [("key", "c")])]))
            mode = "compose"
        elif name == "Type" and mode == "nit" and arg == "r":
            out.append(("reload", cx, [(t, [("key", "r")])]))
        elif name == "Ctrl" and arg == "D" and mode == "nit":
            out.append(("scroll down", cx, [(t, [("key", "⌃"), ("key", "D")])]))
        elif name == "Type" and mode == "nit" and arg == "gg":
            out.append(("to the top", cx, [(t, [("key", "g")]), (t + TYPING, [("key", "g"), ("key", "g")])]))
        elif name == "Ctrl" and arg == "S" and mode == "compose":
            out.append(("send to pi", cx, [(t, [("key", "⌃"), ("key", "S")])]))
            mode = "nit"
    return out


def font(path, size, weight):
    f = ImageFont.truetype(path, size * SS)
    f.set_variation_by_axes([weight if a["name"] == b"Weight" else a["default"] for a in f.get_variation_axes()])
    return f


def rgba(h, a=255):
    return (int(h[1:3], 16), int(h[3:5], 16), int(h[5:7], 16), a)


def bbox(d, s, f):
    l, t, r, b = d.textbbox((0, 0), s, font=f)
    return r - l, b - t, l, t


def card(keys, caption, cx):
    """Draws keys as chips in nit's dark colors, then what they do, low and centered on cx."""
    ov = Image.new("RGBA", (W * SS, H * SS), (0, 0, 0, 0))
    d = ImageDraw.Draw(ov)
    fk, fc = font(MONO, 21, 600), font(SF, 20, 500)
    chip_h, pad, gap = 35 * SS, 12 * SS, 5 * SS
    parts = [(kind, s, max(33 * SS, bbox(d, s, fk)[0] + 17 * SS) if kind == "key" else bbox(d, s, fk)[0])
             for kind, s in keys]
    cw, ch, cl, ct = bbox(d, caption, fc)
    w = sum(pw for _, _, pw in parts) + gap * (len(parts) - 1) + 13 * SS + cw + 2 * pad
    h = chip_h + 2 * 9 * SS
    x0, y1 = cx * SS - w // 2, (H - 47) * SS
    y0 = y1 - h
    d.rounded_rectangle((x0, y0, x0 + w, y1), radius=9 * SS, fill=rgba("#161b22", 240), outline=rgba("#30363d"),
                        width=2 * SS)
    x, cy = x0 + pad, y0 + h // 2
    for kind, s, pw in parts:
        tw, th, l, t = bbox(d, s, fk)
        if kind == "key":
            d.rounded_rectangle((x, cy - chip_h // 2, x + pw, cy + chip_h // 2), radius=6 * SS,
                                fill=rgba("#0d1117"), outline=rgba("#4493f8"), width=2 * SS)
            d.text((x + (pw - tw) // 2 - l, cy - th // 2 - t), s, font=fk, fill=rgba("#e6edf3"))
        else:
            d.text((x - l, cy - th // 2 - t), s, font=fk, fill=rgba("#e6edf3"))
        x += pw + gap
    d.text((x + 13 * SS - gap - cl, cy - ch // 2 - ct), caption, font=fc, fill=rgba("#9198a1"))
    # A soft shadow beneath, then all of it at the video's size.
    sh = Image.new("RGBA", ov.size, (0, 0, 0, 0))
    sh.putalpha(ov.getchannel("A").point(lambda v: v // 2))
    sh = sh.transform(sh.size, Image.AFFINE, (1, 0, 0, 0, 1, -5 * SS)).filter(ImageFilter.GaussianBlur(11 * SS))
    sh.alpha_composite(ov)
    return sh.resize((W, H), Image.LANCZOS)


def draw(lines, video, out):
    """Draws each key group onto video as out, fading in and out, the next replacing the last."""
    dur = subprocess.check_output(["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0",
                                   str(video)], text=True).strip()
    gs = groups(commands(lines, float(dur)))
    tmp = Path(tempfile.mkdtemp())
    inputs, chain, prev = [], [], "0:v"
    for gi, (caption, cx, steps) in enumerate(gs):
        end = steps[-1][0] + HOLD
        if gi + 1 < len(gs):
            end = min(end, gs[gi + 1][2][0][0])
        for si, (t, keys) in enumerate(steps):
            n = len(inputs) // 6
            stop = steps[si + 1][0] if si + 1 < len(steps) else end
            png = tmp / f"{n}.png"
            card(keys, caption, cx).save(png)
            inputs += ["-loop", "1", "-t", dur, "-i", str(png)]
            fades = []
            if si == 0:
                fades.append(f"fade=t=in:st={t:.3f}:d={FADE_IN}:alpha=1")
            if si == len(steps) - 1:
                fades.append(f"fade=t=out:st={max(t, stop - FADE_OUT):.3f}:d={FADE_OUT}:alpha=1")
            chain.append(f"[{n + 1}:v]{','.join(['format=rgba'] + fades)}[k{n}]")
            chain.append(f"[{prev}][k{n}]overlay=enable='gte(t,{t:.3f})*lt(t,{stop:.3f})':eof_action=pass[v{n}]")
            prev = f"v{n}"
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-i", str(video), *inputs, "-filter_complex", ";".join(chain),
                    "-map", f"[{prev}]", "-c:v", "libx264", "-crf", "18", "-pix_fmt", "yuv420p", str(out)], check=True)


if __name__ == "__main__":
    draw(record(), DEMO / "pi.mp4", DEMO / "pi-keys.mp4")
