# Нужен Pillow: uvx --with pillow python tools/make-worker-icon.py (выход — cmd/partscaster-worker/icon.ico и icon.png).
# Иконка partscaster-worker: стиль приложения (тёмный скруглённый квадрат, фаска, янтарный градиент),
# мотив — шестерёнка (работяга-демон) с тёмным «плей»-треугольником в центре.
import math
from PIL import Image, ImageDraw

S = 2048  # рисуем с запасом, вниз — к 1024 (сглаживание)

img = Image.new("RGBA", (S, S), (0, 0, 0, 0))
d = ImageDraw.Draw(img)

# фон: скруглённый квадрат
bg0, bg1 = (13, 13, 18), (30, 30, 40)
r = int(S * 0.22)
for y in range(S):  # вертикальный градиент фона — по строкам
    t = y / S
    c = tuple(int(bg0[i] + (bg1[i] - bg0[i]) * t) for i in range(3)) + (255,)
    d.line([(0, y), (S, y)], fill=c)
mask = Image.new("L", (S, S), 0)
ImageDraw.Draw(mask).rounded_rectangle([0, 0, S - 1, S - 1], radius=r, fill=255)
img.putalpha(mask)

# фаска: светлая сверху-слева, тёмная снизу-справа
bev = Image.new("RGBA", (S, S), (0, 0, 0, 0))
bd = ImageDraw.Draw(bev)
w = int(S * 0.018)
bd.rounded_rectangle([w // 2, w // 2, S - 1 - w // 2, S - 1 - w // 2], radius=r - w // 2,
                     outline=(86, 86, 106, 200), width=w)   # светлая
bd.rounded_rectangle([w, w, S - 1 - w, S - 1 - w], radius=r - w,
                     outline=(16, 16, 21, 220), width=w)     # тёмная
img.alpha_composite(bev)

# шестерёнка: янтарный градиент (как «плей» приложения), 8 зубьев
cx = cy = S / 2
R_out, R_tooth, R_hole = S * 0.36, S * 0.30, S * 0.145
teeth = 8
pts = []
for i in range(teeth * 2):
    ang = math.pi * i / teeth - math.pi / 2
    rad = R_out if i % 2 == 0 else R_tooth
    # зуб — не точка, а дуга: каждая вершина раздваивается
    for da in (-0.16, 0.16):
        a = ang + da
        pts.append((cx + rad * math.cos(a), cy + rad * math.sin(a)))
gear = Image.new("RGBA", (S, S), (0, 0, 0, 0))
gd = ImageDraw.Draw(gear)
gd.polygon(pts, fill=(255, 210, 122, 255))
gd.ellipse([cx - R_tooth, cy - R_tooth, cx + R_tooth, cy + R_tooth], fill=(255, 210, 122, 255))
# градиент шестерёнки: янтарная сверху → тёмно-золотая снизу, через вычитание полупрозрачного
grad = Image.new("L", (S, S), 0)
gdr = ImageDraw.Draw(grad)
for y in range(S):
    gdr.line([(0, y), (S, y)], fill=int(255 * (y / S) * 0.55))
dark = Image.new("RGBA", (S, S), (90, 55, 10, 255))
gear = Image.composite(Image.composite(dark, gear, grad), gear, gear.split()[3])
img.alpha_composite(gear)

# тёмный круг в центре шестерёнки + «плей»-треугольник (перекликается с приложением)
d = ImageDraw.Draw(img)
d.ellipse([cx - R_hole, cy - R_hole, cx + R_hole, cy + R_hole], fill=(13, 13, 18, 255))
tri = int(R_hole * 0.62)
d.polygon([(cx - tri * 0.5, cy - tri), (cx - tri * 0.5, cy + tri), (cx + tri, cy)],
          fill=(255, 190, 61, 255))

img = img.resize((1024, 1024), Image.LANCZOS)
img.save("C:/Users/Dmitry/AppData/Local/Temp/icon/worker-icon.png")
img.save("C:/Users/Dmitry/AppData/Local/Temp/icon/icon.ico", sizes=[(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (16, 16)])
print("ok")
