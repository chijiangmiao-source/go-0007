package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func CanonicalJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, Errorf(CodeValidation, "canonical json failed: %v", err)
	}
	return b, nil
}

func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func HashAny(v any) (string, error) {
	b, err := CanonicalJSON(v)
	if err != nil {
		return "", err
	}
	return HashBytes(b), nil
}

func MustHashAny(v any) string {
	h, err := HashAny(v)
	if err != nil {
		panic(err)
	}
	return h
}
