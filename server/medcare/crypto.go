package medcare

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "errors"
    "io"
    "os"

    "smallgo/server/sysconfig"

    "gorm.io/gorm"
)

func encryptionKey(db *gorm.DB) ([]byte, error) {
    if raw := os.Getenv("MEDCARE_DATA_KEY"); raw != "" {
        sum := sha256.Sum256([]byte(raw))
        return sum[:], nil
    }
    secret, err := sysconfig.GetConfig(db, "jwt_secret", 0)
    if err != nil || secret == "" {
        return nil, errors.New("missing encryption key")
    }
    sum := sha256.Sum256([]byte("medcare-channel:" + secret))
    return sum[:], nil
}

func encryptString(db *gorm.DB, plain string) (string, error) {
    key, err := encryptionKey(db)
    if err != nil {
        return "", err
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }
    sealed := gcm.Seal(nonce, nonce, []byte(plain), nil)
    return base64.RawStdEncoding.EncodeToString(sealed), nil
}

func decryptString(db *gorm.DB, encoded string) (string, error) {
    key, err := encryptionKey(db)
    if err != nil {
        return "", err
    }
    raw, err := base64.RawStdEncoding.DecodeString(encoded)
    if err != nil {
        return "", err
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return "", err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }
    if len(raw) < gcm.NonceSize() {
        return "", errors.New("invalid encrypted value")
    }
    out, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
    if err != nil {
        return "", err
    }
    return string(out), nil
}

func encryptBytes(db *gorm.DB, plain []byte) ([]byte, error) {
    key, err := encryptionKey(db)
    if err != nil {
        return nil, err
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return nil, err
    }
    return gcm.Seal(nonce, nonce, plain, nil), nil
}

func decryptBytes(db *gorm.DB, sealed []byte) ([]byte, error) {
    key, err := encryptionKey(db)
    if err != nil {
        return nil, err
    }
    block, err := aes.NewCipher(key)
    if err != nil {
        return nil, err
    }
    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return nil, err
    }
    if len(sealed) < gcm.NonceSize() {
        return nil, errors.New("invalid encrypted image")
    }
    return gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], nil)
}
