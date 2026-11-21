#!/usr/bin/env python3
"""生成 smallgo 应用图标：3D 立体玻璃立方体。

输出（在仓库根目录运行：python3 scripts/generate_icon.py）：
  fnpack/ICON.PNG / fnpack/ICON_256.PNG   fnOS 打包图标
  web/public/favicon.svg                  矢量版（手工配套维护）
  web/public/favicon-512/-192/-32/-16.png web 站点图标
  web/public/apple-touch-icon.png         iOS 桌面图标（180，白底）
  web/public/favicon.ico                  多尺寸 ico

设计：靛→紫渐变玻璃底板 + 顶部光泽 + 内侧高光描边；
主体为 3D 等距玻璃立方体（半透明三面 + 亮边 + 镜面高光 + 投影），
寓意 smallgo「应用积木基座」。4096 超采样后缩放，保证边缘光滑。
"""

from pathlib import Path

from PIL import Image, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parent.parent
S = 4096  # 超采样画布
MASTER = 1024

# 品牌渐变（与 web 主题 brand 色一致）
GRAD_TOP = (99, 102, 241)    # indigo-500
GRAD_MID = (139, 92, 246)    # violet-500
GRAD_BOT = (168, 85, 247)    # purple-500


def lerp(a, b, t):
    return tuple(int(a[i] + (b[i] - a[i]) * t) for i in range(3))


def vertical_gradient(size, stops):
    """stops: [(pos0..1, rgb), ...] 生成垂直渐变图。"""
    w, h = size
    img = Image.new("RGB", (1, h))
    px = img.load()
    for y in range(h):
        t = y / (h - 1)
        for (p0, c0), (p1, c1) in zip(stops, stops[1:]):
            if p0 <= t <= p1:
                tt = (t - p0) / (p1 - p0) if p1 > p0 else 0
                px[0, y] = lerp(c0, c1, tt)
                break
        else:
            px[0, y] = stops[-1][1]
    return img.resize((w, h))


def radial_alpha_layer(size, center, radius, color, peak_alpha, falloff=2.0):
    """柔和高光/阴影层：椭圆径向渐变（alpha 由中心向外衰减）。"""
    w, h = size
    layer = Image.new("RGBA", (w, h), (0, 0, 0, 0))
    px = layer.load()
    cx, cy = center
    rx, ry = radius
    small = 4  # 先在小图上算渐变再放大，提速
    sw, sh = max(w // small, 1), max(h // small, 1)
    sl = Image.new("RGBA", (sw, sh), (0, 0, 0, 0))
    spx = sl.load()
    for y in range(sh):
        for x in range(sw):
            dx = (x * small - cx) / rx
            dy = (y * small - cy) / ry
            d = (dx * dx + dy * dy) ** 0.5
            if d < 1:
                a = int(peak_alpha * (1 - d) ** falloff)
                spx[x, y] = (*color, a)
    layer = sl.resize((w, h), Image.BILINEAR)
    return layer


def tile_mask(size, radius):
    m = Image.new("L", (size, size), 0)
    ImageDraw.Draw(m).rounded_rectangle([0, 0, size - 1, size - 1], radius=radius, fill=255)
    return m


def build_base_square(size):
    """不透明渐变底板 + 光影（未裁圆角），供底板与 apple-touch 复用。"""
    img = vertical_gradient((size, size), [(0.0, GRAD_TOP), (0.55, GRAD_MID), (1.0, GRAD_BOT)]).convert("RGBA")

    # 左上环境光 / 底部暗角，塑造立体
    img.alpha_composite(radial_alpha_layer((size, size), (int(size * 0.22), int(size * 0.08)),
                                           (int(size * 0.62), int(size * 0.5)), (199, 210, 254), 70, 1.8))
    img.alpha_composite(radial_alpha_layer((size, size), (int(size * 0.5), int(size * 1.08)),
                                           (int(size * 0.75), int(size * 0.45)), (49, 46, 129), 90, 1.6))
    return img


def draw_cube(size):
    """3D 等距玻璃立方体图层（含投影）。"""
    layer = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)

    cx, cy = size * 0.5, size * 0.485
    R = size * 0.235  # 外接六边形半径
    hx = R * 0.866
    hy = R * 0.5

    P1 = (cx, cy - R)          # 上
    P2 = (cx + hx, cy - hy)    # 右上
    P3 = (cx + hx, cy + hy)    # 右下
    P4 = (cx, cy + R)          # 下
    P5 = (cx - hx, cy + hy)    # 左下
    P6 = (cx - hx, cy - hy)    # 左上
    C = (cx, cy)

    # 落地投影
    shadow = radial_alpha_layer((size, size), (cx, int(cy + R * 1.02)),
                                (int(R * 0.95), int(R * 0.30)), (30, 27, 75), 110, 1.5)
    layer.alpha_composite(shadow)

    def face_gradient(points, c_from, c_to, alpha, vertical=True):
        """多边形内垂直/水平渐变贴图。"""
        xs = [p[0] for p in points]
        ys = [p[1] for p in points]
        box = (int(min(xs)), int(min(ys)), int(max(xs)) + 1, int(max(ys)) + 1)
        bw, bh = box[2] - box[0], box[3] - box[1]
        if bw <= 0 or bh <= 0:
            return
        if vertical:
            grad = vertical_gradient((bw, bh), [(0.0, c_from), (1.0, c_to)])
        else:
            grad = vertical_gradient((bw, bh), [(0.0, c_from), (1.0, c_to)]).rotate(90, expand=True)
        mask = Image.new("L", (bw, bh), 0)
        ImageDraw.Draw(mask).polygon([(p[0] - box[0], p[1] - box[1]) for p in points], fill=alpha)
        solid = Image.new("RGBA", (bw, bh), (0, 0, 0, 0))
        solid.paste(grad, (0, 0))
        layer.alpha_composite(Image.composite(solid, Image.new("RGBA", (bw, bh), (0, 0, 0, 0)), mask), box[:2])

    # 三面：玻璃半透明，让底板渐变透出来
    face_gradient([P6, P1, P2, C], (250, 250, 255), (214, 219, 255), 225)          # 顶面（亮）
    face_gradient([P6, C, P4, P5], (124, 58, 237), (91, 33, 182), 200)             # 左面
    face_gradient([C, P2, P3, P4], (76, 29, 149), (67, 56, 202), 215)              # 右面（深）

    # 棱线：外缘亮边 + 内棱
    edge = int(size * 0.0045)
    d.line([P6, P1, P2], fill=(255, 255, 255, 200), width=edge * 2, joint="curve")
    d.line([P2, C, P6], fill=(255, 255, 255, 130), width=edge, joint="curve")
    d.line([C, P4], fill=(255, 255, 255, 90), width=edge)
    d.line([P6, P5, P4], fill=(255, 255, 255, 70), width=edge)
    d.line([P2, P3, P4], fill=(255, 255, 255, 70), width=edge)

    # 右面斜向反光条（玻璃质感）
    streak = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    sd = ImageDraw.Draw(streak)
    sd.polygon([(C[0], C[1] + R * 0.08), (P2[0] - hx * 0.18, P2[1] + hy * 0.22),
                (P2[0] - hx * 0.02, P2[1] + hy * 0.30), (C[0] + hx * 0.16, C[1] + R * 0.55)],
               fill=(255, 255, 255, 36))
    streak = streak.filter(ImageFilter.GaussianBlur(size * 0.006))
    layer.alpha_composite(streak)

    # 顶面镜面高光点
    spec = radial_alpha_layer((size, size), (int(cx - hx * 0.28), int(cy - R * 0.52)),
                              (int(R * 0.28), int(R * 0.16)), (255, 255, 255), 160, 2.2)
    layer.alpha_composite(spec)

    return layer


def build_icon(master=MASTER, square=False):
    size = S
    radius = int(size * 0.225)

    img = build_base_square(size)

    # 顶部玻璃光泽（大椭圆高光，贴着顶边）
    gloss = radial_alpha_layer((size, size), (int(size * 0.5), int(size * 0.04)),
                               (int(size * 0.78), int(size * 0.42)), (255, 255, 255), 90, 1.8)
    img.alpha_composite(gloss)

    # 斜向 sheen：左上→右下的柔光带
    sheen = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    sd = ImageDraw.Draw(sheen)
    sd.polygon([(0, int(size * 0.52)), (int(size * 0.52), 0), (int(size * 0.78), 0), (0, int(size * 0.78))],
               fill=(255, 255, 255, 26))
    sheen = sheen.filter(ImageFilter.GaussianBlur(size * 0.02))
    img.alpha_composite(sheen)

    # 主体立方体
    img.alpha_composite(draw_cube(size))

    if not square:
        mask = tile_mask(size, radius)
        out = Image.new("RGBA", (size, size), (0, 0, 0, 0))
        out.paste(img, (0, 0), mask)
        img = out
        # 内侧亮边描边
        d = ImageDraw.Draw(img)
        d.rounded_rectangle([size * 0.004, size * 0.004, size * 0.996, size * 0.996],
                            radius=radius - size * 0.004, outline=(255, 255, 255, 70), width=int(size * 0.006))

    return img.resize((master, master), Image.LANCZOS)


def main():
    master = build_icon()
    pub = ROOT / "web" / "public"
    fnpack = ROOT / "fnpack"

    for name, px in [("favicon-512.png", 512), ("favicon-192.png", 192),
                     ("favicon-32.png", 32), ("favicon-16.png", 16)]:
        master.resize((px, px), Image.LANCZOS).save(pub / name)

    # iOS 桌面图标：不透明方形，180 标准尺寸
    apple = build_icon(square=True)
    apple.resize((180, 180), Image.LANCZOS).save(pub / "apple-touch-icon.png")

    # fnOS 打包图标（ICON.PNG 家族惯例 64x64；ICON_256.PNG 标准件 256）
    master.resize((64, 64), Image.LANCZOS).save(fnpack / "ICON.PNG")
    master.resize((256, 256), Image.LANCZOS).save(fnpack / "ICON_256.PNG")

    # 多尺寸 ico
    master.save(pub / "favicon.ico", sizes=[(16, 16), (32, 32), (48, 48)])

    print("icons generated under", ROOT)


if __name__ == "__main__":
    main()
