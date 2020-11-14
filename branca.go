// Package branca implements the branca token specification.
package branca

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/eknkc/basex"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	version byte   = 0xBA // Branca magic byte
	base62  string = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

var (
	// ErrInvalidToken indicates an invalid token.
	ErrInvalidToken = errors.New("invalid base62 token")
	// ErrInvalidTokenVersion indicates an invalid token version.
	ErrInvalidTokenVersion = errors.New("invalid token version")
	// ErrBadKeyLength indicates a bad key length.
	ErrBadKeyLength = errors.New("bad key length")
)

// ErrExpiredToken indicates an expired token.
type ErrExpiredToken struct {
	// Time is the token expiration time.
	Time time.Time
}

func (e *ErrExpiredToken) Error() string {
	delta := time.Unix(time.Now().Unix(), 0).Sub(time.Unix(e.Time.Unix(), 0))
	return fmt.Sprintf("token is expired by %v", delta)
}

// Branca holds a key of exactly 32 bytes. The nonce and timestamp are used for acceptance tests.
type Branca struct {
	Key       string
	nonce     string
	ttl       uint32
	timestamp uint32
}

// SetTTL sets a Time To Live on the token for valid tokens.
func (b *Branca) SetTTL(ttl uint32) {
	b.ttl = ttl
}

// setTimeStamp sets a timestamp for testing.
func (b *Branca) setTimeStamp(timestamp uint32) {
	b.timestamp = timestamp
}

// setNonce sets a nonce for testing.
func (b *Branca) setNonce(nonce string) {
	b.nonce = nonce
}

// NewBranca creates a *Branca struct.
func NewBranca(key string) (b *Branca) {
	return &Branca{
		Key: key,
	}
}

// EncodeBinary encodes the data matching the format:
// Version (byte) || Timestamp ([4]byte) || Nonce ([24]byte) || Ciphertext ([]byte) || Tag ([16]byte)
func (b *Branca) EncodeBinary(data []byte) (string, error) {
	var timestamp uint32
	var nonce []byte
	if b.timestamp == 0 {
		b.timestamp = uint32(time.Now().Unix())
	}
	timestamp = b.timestamp

	if len(b.nonce) == 0 {
		nonce = make([]byte, 24)
		if _, err := rand.Read(nonce); err != nil {
			return "", err
		}
	} else {
		noncebytes, err := hex.DecodeString(b.nonce)
		if err != nil {
			return "", ErrInvalidToken
		}
		nonce = noncebytes
	}

	key := bytes.NewBufferString(b.Key).Bytes()

	timeBuffer := make([]byte, 4)
	binary.BigEndian.PutUint32(timeBuffer, timestamp)
	header := append(timeBuffer, nonce...)
	header = append([]byte{version}, header...)

	xchacha, err := chacha20poly1305.NewX(key)
	if err != nil {
		return "", ErrBadKeyLength
	}

	ciphertext := xchacha.Seal(nil, nonce, data, header)

	token := append(header, ciphertext...)
	base62, err := basex.NewEncoding(base62)
	if err != nil {
		return "", err
	}
	return base62.Encode(token), nil
}

// EncodeToString encodes the string data.
func (b *Branca) EncodeToString(data string) (string, error) {
	return b.EncodeBinary(bytes.NewBufferString(data).Bytes())
}

// DecodeToBinary decodes the data.
func (b *Branca) DecodeToBinary(data string) ([]byte, error) {
	if len(data) < 62 {
		return nil, fmt.Errorf("%w: length is less than 62", ErrInvalidToken)
	}
	base62, err := basex.NewEncoding(base62)
	if err != nil {
		return nil, fmt.Errorf("%v", err)
	}
	token, err := base62.Decode(data)
	if err != nil {
		return nil, ErrInvalidToken
	}
	header := token[:29]
	ciphertext := token[29:]
	tokenversion := header[0]
	timestamp := binary.BigEndian.Uint32(header[1:5])
	nonce := header[5:]

	if tokenversion != version {
		return nil, fmt.Errorf("%w: got %#X but expected %#X", ErrInvalidTokenVersion, tokenversion, version)
	}

	key := bytes.NewBufferString(b.Key).Bytes()

	xchacha, err := chacha20poly1305.NewX(key)
	if err != nil {
		return nil, ErrBadKeyLength
	}
	payload, err := xchacha.Open(nil, nonce, ciphertext, header)
	if err != nil {
		return nil, err
	}

	if b.ttl != 0 {
		future := int64(timestamp + b.ttl)
		now := time.Now().Unix()
		if future < now {
			return nil, &ErrExpiredToken{Time: time.Unix(future, 0)}
		}
	}

	return payload, nil
}

// DecodeToString decodes the data to string.
func (b *Branca) DecodeToString(data string) (string, error) {
	payload, err := b.DecodeToBinary(data)
	if err != nil {
		return "", err
	}

	payloadString := bytes.NewBuffer(payload).String()
	return payloadString, nil
}
