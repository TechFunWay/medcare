package medcare

import (
    "bytes"
    "image"
    "image/color"
    "image/draw"
    _ "image/gif"
    "image/jpeg"
    "image/png"
    "math"
)

const (
    imageMaxDimension   = 2560 // 原图长边最大 2560px，清楚但不至于太大
    imageJPEGQuality    = 90
    imageThumbDimension = 420 // 列表缩略图长边 420px
    imageThumbQuality   = 80
)

// compressAndThumb 对上传图片做「限制长边 + 重编码」的轻量压缩，并生成列表缩略图。
// GIF（可能是动图）与 WebP（标准库无解码器）保留原图；前端上传前会先把常见图片压成 JPEG。
func compressAndThumb(raw []byte, mime string) (main []byte, mainMime string, thumb []byte) {
    if mime == "image/gif" || mime == "image/webp" {
        return raw, mime, jpegThumbFromRaw(raw)
    }
    src, _, err := image.Decode(bytes.NewReader(raw))
    if err != nil {
        return raw, mime, nil
    }
    b := src.Bounds()
    resized := false
    mainImg := src
    if b.Dx() > imageMaxDimension || b.Dy() > imageMaxDimension {
        mainImg = scaleImage(src, imageMaxDimension)
        resized = true
    }
    var buf bytes.Buffer
    outMime := mime
    if mime == "image/png" {
        outMime = "image/png"
        if err := png.Encode(&buf, mainImg); err != nil {
            return raw, mime, jpegThumbFromImage(mainImg)
        }
    } else {
        outMime = "image/jpeg"
        if err := jpeg.Encode(&buf, mainImg, &jpeg.Options{Quality: imageJPEGQuality}); err != nil {
            return raw, mime, jpegThumbFromImage(mainImg)
        }
    }
    main = buf.Bytes()
    // 小图重编码可能反而变大，保清晰优先，仍用原图。
    if !resized && len(main) >= len(raw) {
        main = raw
        outMime = mime
    }
    return main, outMime, jpegThumbFromImage(mainImg)
}

func jpegThumbFromRaw(raw []byte) []byte {
    src, _, err := image.Decode(bytes.NewReader(raw))
    if err != nil {
        return nil
    }
    return jpegThumbFromImage(src)
}

func jpegThumbFromImage(src image.Image) []byte {
    thumbImg := scaleImage(src, imageThumbDimension)
    b := thumbImg.Bounds()
    bg := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
    draw.Draw(bg, bg.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
    draw.Draw(bg, bg.Bounds(), thumbImg, b.Min, draw.Over)
    var buf bytes.Buffer
    if err := jpeg.Encode(&buf, bg, &jpeg.Options{Quality: imageThumbQuality}); err != nil {
        return nil
    }
    return buf.Bytes()
}

// scaleImage 使用双线性插值缩放，避免最近邻那种锯齿和糊字。
func scaleImage(src image.Image, maxDim int) image.Image {
    b := src.Bounds()
    sw, sh := b.Dx(), b.Dy()
    if sw <= 0 || sh <= 0 {
        return src
    }
    scale := float64(maxDim) / float64(maxInt(sw, sh))
    if scale >= 1 {
        return src
    }
    dw := maxInt(1, int(math.Round(float64(sw)*scale)))
    dh := maxInt(1, int(math.Round(float64(sh)*scale)))
    dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
    for y := 0; y < dh; y++ {
        sy := (float64(y)+0.5)*float64(sh)/float64(dh) - 0.5
        y0 := int(math.Floor(sy))
        fy := sy - float64(y0)
        if y0 < 0 {
            y0 = 0
            fy = 0
        }
        y1 := y0 + 1
        if y1 >= sh {
            y1 = sh - 1
        }
        for x := 0; x < dw; x++ {
            sx := (float64(x)+0.5)*float64(sw)/float64(dw) - 0.5
            x0 := int(math.Floor(sx))
            fx := sx - float64(x0)
            if x0 < 0 {
                x0 = 0
                fx = 0
            }
            x1 := x0 + 1
            if x1 >= sw {
                x1 = sw - 1
            }
            r00, g00, b00, a00 := src.At(b.Min.X+x0, b.Min.Y+y0).RGBA()
            r10, g10, b10, a10 := src.At(b.Min.X+x1, b.Min.Y+y0).RGBA()
            r01, g01, b01, a01 := src.At(b.Min.X+x0, b.Min.Y+y1).RGBA()
            r11, g11, b11, a11 := src.At(b.Min.X+x1, b.Min.Y+y1).RGBA()
            r := bilerp(r00, r10, r01, r11, fx, fy) >> 8
            g := bilerp(g00, g10, g01, g11, fx, fy) >> 8
            bb := bilerp(b00, b10, b01, b11, fx, fy) >> 8
            a := bilerp(a00, a10, a01, a11, fx, fy) >> 8
            dst.SetRGBA(x, y, color.RGBA{R: uint8(r), G: uint8(g), B: uint8(bb), A: uint8(a)})
        }
    }
    return dst
}

func bilerp(c00, c10, c01, c11 uint32, fx, fy float64) uint32 {
    top := float64(c00)*(1-fx) + float64(c10)*fx
    bottom := float64(c01)*(1-fx) + float64(c11)*fx
    return uint32(top*(1-fy) + bottom*fy)
}

func maxInt(a, b int) int {
    if a > b {
        return a
    }
    return b
}
