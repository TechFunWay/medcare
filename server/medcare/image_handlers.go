package medcare

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "path/filepath"
    "strings"
    "time"

    "smallgo/server/audit"
    "smallgo/server/response"
    "smallgo/server/utils"

    "github.com/gin-gonic/gin"
    "gorm.io/gorm"
)

// 图片分类：药品、处方单、治疗单、检查报告、其他。
const (
    ImageCatMedicine     = "medicine"
    ImageCatPrescription = "prescription"
    ImageCatTreatment    = "treatment"
    ImageCatReport       = "report"
    ImageCatOther        = "other"
)

var imageCategories = map[string]bool{
    ImageCatMedicine: true, ImageCatPrescription: true, ImageCatTreatment: true,
    ImageCatReport: true, ImageCatOther: true,
}

const imageMaxBytes = 20 << 20 // 20 MiB

type imageDTO struct {
    ID            uint      `json:"id"`
    BookID        uint      `json:"book_id"`
    Title         string    `json:"title"`
    Category      string    `json:"category"`
    MedicineID    uint      `json:"medicine_id"`
    Tags          []string  `json:"tags"`
    Note          string    `json:"note"`
    Mime          string    `json:"mime"`
    Size          int64     `json:"size"`
    CreatedBy     uint      `json:"created_by"`
    CreatedByName string    `json:"created_by_name"`
    CreatedAt     time.Time `json:"created_at"`
    UpdatedAt     time.Time `json:"updated_at"`
}

func imageToDTO(db *gorm.DB, img BookImage) imageDTO {
    tags := []string{}
    _ = decodeJSON(img.Tags, &tags)
    return imageDTO{
        ID: img.ID, BookID: img.BookID, Title: img.Title, Category: img.Category, MedicineID: img.MedicineID,
        Tags: tags, Note: img.Note, Mime: img.Mime, Size: img.Size,
        CreatedBy: img.CreatedBy, CreatedByName: userNameOrEmpty(db, img.CreatedBy),
        CreatedAt: img.CreatedAt, UpdatedAt: img.UpdatedAt,
    }
}

func parseImageTags(raw string) []string {
    raw = strings.TrimSpace(raw)
    if raw == "" {
        return []string{}
    }
    var arr []string
    if strings.HasPrefix(raw, "[") {
        _ = json.Unmarshal([]byte(raw), &arr)
    } else {
        arr = strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '，' || r == '、' || r == ' ' || r == '\n' })
    }
    out := make([]string, 0, len(arr))
    seen := map[string]bool{}
    for _, t := range arr {
        t = strings.TrimSpace(t)
        if t == "" || len([]rune(t)) > 20 || seen[t] {
            continue
        }
        seen[t] = true
        out = append(out, t)
        if len(out) >= 10 {
            break
        }
    }
    return out
}

func handleListImages(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        page := utils.Atoi(c.Query("page"), 1)
        pageSize := utils.Atoi(c.Query("pageSize"), 24)
        if pageSize > 100 {
            pageSize = 100
        }
        q := db.Model(&BookImage{}).Where("book_id = ?", contextBookID(c))
        if cat := strings.TrimSpace(c.Query("category")); cat != "" && cat != "all" {
            q = q.Where("category = ?", cat)
        }
        if mid := utils.Atoi(c.Query("medicine_id"), 0); mid > 0 {
            q = q.Where("medicine_id = ?", mid)
        }
        if kw := strings.TrimSpace(c.Query("q")); kw != "" {
            like := "%" + kw + "%"
            q = q.Where("title LIKE ? OR note LIKE ? OR tags LIKE ?", like, like, like)
        }
        var total int64
        if err := q.Count(&total).Error; err != nil {
            response.ErrorInternal(c, "读取图片失败")
            return
        }
        var rows []BookImage
        if err := q.Select("id, book_id, title, category, medicine_id, tags, note, mime, size, created_by, created_at, updated_at").
            Order("created_at DESC").Offset(utils.Offset(page, pageSize)).Limit(pageSize).Find(&rows).Error; err != nil {
            response.ErrorInternal(c, "读取图片失败")
            return
        }
        items := make([]imageDTO, 0, len(rows))
        for _, img := range rows {
            items = append(items, imageToDTO(db, img))
        }
        response.SuccessPage(c, items, total, page, pageSize)
    }
}

func handleUploadImages(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, imageMaxBytes+1<<20)
        form, err := c.MultipartForm()
        if err != nil {
            response.ErrorBadRequest(c, "请上传图片")
            return
        }
        files := form.File["files"]
        if len(files) == 0 {
            files = form.File["file"]
        }
        if len(files) == 0 {
            response.ErrorBadRequest(c, "请上传图片")
            return
        }
        // 挂在药品下的图片：校验药品属于本用药本，分类默认按药品照片走。
        medicineID := uint(utils.Atoi(c.PostForm("medicine_id"), 0))
        if medicineID > 0 {
            var cnt int64
            db.Model(&Medicine{}).Where("id = ? AND book_id = ? AND archived_at IS NULL", medicineID, contextBookID(c)).Count(&cnt)
            if cnt == 0 {
                response.ErrorBadRequest(c, "药品不存在")
                return
            }
        }
        category := strings.TrimSpace(c.PostForm("category"))
        if category == "" {
            category = ImageCatOther
            if medicineID > 0 {
                category = ImageCatMedicine
            }
        }
        if !imageCategories[category] {
            response.ErrorBadRequest(c, "图片分类无效")
            return
        }
        tags := parseImageTags(c.PostForm("tags"))
        note := strings.TrimSpace(c.PostForm("note"))
        titleInput := strings.TrimSpace(c.PostForm("title"))
        userID := currentUserID(c)
        now := nowUTC()
        saved := make([]imageDTO, 0, len(files))
        for fi, fh := range files {
            if fh.Size <= 0 || fh.Size > imageMaxBytes {
                response.ErrorBadRequest(c, fmt.Sprintf("图片过大（最大 %d MB）", imageMaxBytes>>20))
                return
            }
            f, err := fh.Open()
            if err != nil {
                response.ErrorBadRequest(c, "读取图片失败")
                return
            }
            raw, err := io.ReadAll(io.LimitReader(f, imageMaxBytes+1))
            f.Close()
            if err != nil || len(raw) == 0 {
                response.ErrorBadRequest(c, "读取图片失败")
                return
            }
            if len(raw) > imageMaxBytes {
                response.ErrorBadRequest(c, fmt.Sprintf("图片过大（最大 %d MB）", imageMaxBytes>>20))
                return
            }
            mime := http.DetectContentType(raw)
            switch mime {
            case "image/jpeg", "image/png", "image/gif", "image/webp":
            default:
                response.ErrorBadRequest(c, "仅支持 JPEG、PNG、GIF 或 WebP 图片")
                return
            }
            mainBytes, mainMime, thumbBytes := compressAndThumb(raw, mime)
            encrypted, err := encryptBytes(db, mainBytes)
            if err != nil {
                response.ErrorInternal(c, "保存图片失败")
                return
            }
            var thumbEncrypted []byte
            var thumbMime string
            if len(thumbBytes) > 0 {
                if sealed, err := encryptBytes(db, thumbBytes); err == nil {
                    thumbEncrypted = sealed
                    thumbMime = "image/jpeg"
                }
            }
            title := titleInput
            if title != "" && len(files) > 1 {
                title = fmt.Sprintf("%s %d", title, fi+1)
            }
            if title == "" {
                title = strings.TrimSuffix(filepath.Base(fh.Filename), filepath.Ext(fh.Filename))
            }
            if title == "" {
                title = "未命名图片"
            }
            if len([]rune(title)) > 100 {
                title = string([]rune(title)[:100])
            }
            img := BookImage{
                BookID: contextBookID(c), Title: title, Category: category, MedicineID: medicineID,
                Tags: encodeJSON(tags), Note: note, ImageEncrypted: encrypted,
                Mime: mainMime, Size: int64(len(mainBytes)), ThumbEncrypted: thumbEncrypted,
                ThumbMime: thumbMime, ThumbSize: int64(len(thumbBytes)), CreatedBy: userID,
                CreatedAt: now, UpdatedAt: now,
            }
            if err := db.Create(&img).Error; err != nil {
                response.ErrorInternal(c, "保存图片失败")
                return
            }
            saved = append(saved, imageToDTO(db, img))
        }
        audit.Log(db, c, "medcare_image_upload", "book_image", saved[0].ID, fmt.Sprintf("%d 张", len(saved)))
        response.Success(c, gin.H{"items": saved})
    }
}

func loadImage(db *gorm.DB, c *gin.Context) (BookImage, bool) {
    id, ok := parseUintParam(c, "id")
    if !ok {
        return BookImage{}, false
    }
    var img BookImage
    if err := db.Where("id = ? AND book_id = ?", id, contextBookID(c)).First(&img).Error; err != nil {
        response.ErrorNotFound(c, "图片不存在")
        return BookImage{}, false
    }
    return img, true
}

func handleImageFile(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        img, ok := loadImage(db, c)
        if !ok {
            return
        }
        raw, err := decryptBytes(db, img.ImageEncrypted)
        if err != nil {
            response.ErrorInternal(c, "读取图片失败")
            return
        }
        mime := img.Mime
        if c.Query("thumb") == "1" {
            if len(img.ThumbEncrypted) > 0 {
                raw, err = decryptBytes(db, img.ThumbEncrypted)
                if err != nil {
                    response.ErrorInternal(c, "读取缩略图失败")
                    return
                }
                mime = img.ThumbMime
            } else {
                // 兼容旧图片：首次访问缩略图时生成并回写，之后直接读缓存。
                if thumb := jpegThumbFromRaw(raw); len(thumb) > 0 {
                    if sealed, err := encryptBytes(db, thumb); err == nil {
                        db.Model(&img).Updates(map[string]interface{}{"thumb_encrypted": sealed, "thumb_mime": "image/jpeg", "thumb_size": int64(len(thumb))})
                        raw, mime = thumb, "image/jpeg"
                    }
                }
            }
        }
        ext := ".jpg"
        switch mime {
        case "image/png":
            ext = ".png"
        case "image/gif":
            ext = ".gif"
        case "image/webp":
            ext = ".webp"
        }
        disposition := "inline"
        if c.Query("download") == "1" {
            disposition = "attachment"
        }
        safe := strings.Map(func(r rune) rune {
            if r == '"' || r == '\\' || r == '\n' || r == '\r' {
                return -1
            }
            return r
        }, img.Title)
        c.Header("Content-Type", mime)
        c.Header("Content-Disposition", fmt.Sprintf("%s; filename=%q; filename*=UTF-8''%s", disposition, "image"+ext, urlQueryEscape(safe+ext)))
        c.Header("Cache-Control", "private, max-age=86400")
        c.Data(http.StatusOK, mime, raw)
    }
}

func urlQueryEscape(s string) string {
    // 只用于 Content-Disposition 的 filename*，避免引入额外依赖。
    var b strings.Builder
    for _, c := range []byte(s) {
        if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '-' || c == '_' {
            b.WriteByte(c)
        } else {
            b.WriteString(fmt.Sprintf("%%%02X", c))
        }
    }
    return b.String()
}

func handleUpdateImage(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        img, ok := loadImage(db, c)
        if !ok {
            return
        }
        var in struct {
            Title    string   `json:"title"`
            Category string   `json:"category"`
            Tags     []string `json:"tags"`
            Note     string   `json:"note"`
        }
        if err := c.ShouldBindJSON(&in); err != nil {
            response.ErrorBadRequest(c, "参数无效")
            return
        }
        title := strings.TrimSpace(in.Title)
        if title == "" || len([]rune(title)) > 100 {
            response.ErrorBadRequest(c, "图片标题应为 1–100 个字符")
            return
        }
        category := strings.TrimSpace(in.Category)
        if category == "" {
            category = img.Category
        }
        if !imageCategories[category] {
            response.ErrorBadRequest(c, "图片分类无效")
            return
        }
        tags := in.Tags
        if tags == nil {
            _ = decodeJSON(img.Tags, &tags)
        }
        clean := make([]string, 0, len(tags))
        seen := map[string]bool{}
        for _, t := range tags {
            t = strings.TrimSpace(t)
            if t == "" || len([]rune(t)) > 20 || seen[t] {
                continue
            }
            seen[t] = true
            clean = append(clean, t)
            if len(clean) >= 10 {
                break
            }
        }
        if err := db.Model(&img).Updates(map[string]interface{}{
            "title": title, "category": category, "tags": encodeJSON(clean),
            "note": strings.TrimSpace(in.Note), "updated_at": nowUTC(),
        }).Error; err != nil {
            response.ErrorInternal(c, "保存失败")
            return
        }
        img.Title, img.Category, img.Tags, img.Note = title, category, encodeJSON(clean), strings.TrimSpace(in.Note)
        response.Success(c, imageToDTO(db, img))
    }
}

func handleDeleteImage(db *gorm.DB) gin.HandlerFunc {
    return func(c *gin.Context) {
        img, ok := loadImage(db, c)
        if !ok {
            return
        }
        if err := db.Delete(&img).Error; err != nil {
            response.ErrorInternal(c, "删除失败")
            return
        }
        // 删掉的图片如果正被当作药品封面，顺手清掉封面，避免列表指向不存在的图。
        db.Model(&Medicine{}).Where("book_id = ? AND cover_image_id = ?", img.BookID, img.ID).
            Updates(map[string]interface{}{"cover_image_id": 0, "updated_at": nowUTC()})
        audit.Log(db, c, "medcare_image_delete", "book_image", img.ID, img.Title)
        response.Success(c, gin.H{"deleted": true})
    }
}
