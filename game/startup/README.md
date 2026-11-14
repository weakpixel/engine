# Startup screen image

`weakpixel-screen.jpg` is the default startup screen (`game.WeakpixelScreen`), embedded in `game`, so it ships with every game. It and `images/weakpixel-screen.png` (the README banner) are smaller copies of the original 1254x1254 artwork, which is not in the repository:

- `weakpixel-screen.jpg`: scaled to 720x720 with Lanczos, JPEG quality 85 (about 50 KB). The screen is drawn letterboxed at window resolution, so the size only sets how sharp it looks.
- `images/weakpixel-screen.png`: scaled to 640x640 with Lanczos, quantized with `pngquant --quality 80-95 --speed 1`, then `oxipng` level 6 (about 165 KB).

To redo them from the original (`pip install pyoxipng pngquant-cli`):

```bash
python3 -c "from PIL import Image; im = Image.open('original.png').convert('RGB'); im.resize((720, 720), Image.LANCZOS).save('game/startup/weakpixel-screen.jpg', 'JPEG', quality=85, optimize=True); im.resize((640, 640), Image.LANCZOS).save('/tmp/banner.png')"
pngquant --quality 80-95 --speed 1 -f -o images/weakpixel-screen.png /tmp/banner.png
python3 -c "import oxipng; oxipng.optimize('images/weakpixel-screen.png', level=6, strip=oxipng.StripChunks.safe())"
```

Keep `WeakpixelScreen().Background` in `game/startup.go` equal to the image's edge colour (the near-black top and sides).
