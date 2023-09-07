package cached

import (
	"crypto/sha1"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/wal-g/tracelog"
	"github.com/wal-g/wal-g/internal/crypto/envelope"
)

type Item struct {
	Object    []byte
	ExpiredAt time.Time
}

func (item *Item) isFresh() bool {
	return item.ExpiredAt.IsZero() || item.ExpiredAt.After(time.Now())
}

type Enveloper struct {
	wrapped    envelope.Enveloper
	locker     sync.RWMutex
	items      map[string]Item
	expiration time.Duration
}

func createKey(s []byte) string { return fmt.Sprintf("%x", sha1.Sum(s)) }

func (enveloper *Enveloper) Name() string {
	return enveloper.wrapped.Name()
}

func (enveloper *Enveloper) ReadEncryptedKey(r io.Reader) ([]byte, error) {
	tracelog.DebugLogger.Println("Exctract encrypted key")
	return enveloper.wrapped.ReadEncryptedKey(r)
}

func (enveloper *Enveloper) DecryptKey(encryptedKey []byte) ([]byte, error) {
	key := createKey(encryptedKey)
	tracelog.DebugLogger.Printf("Decrypt encrypted key %s\n", key)

	enveloper.locker.RLock()
	item, exists := enveloper.items[key]
	enveloper.locker.RUnlock()
	if exists && item.isFresh() {
		tracelog.DebugLogger.Printf("Use cached encrypted key %s \n", key)
		return item.Object, nil
	}

	decryptedKey, err := enveloper.wrapped.DecryptKey(encryptedKey)
	if err != nil {
		if exists {
			tracelog.WarningLogger.Printf("Unable to decrypt a key, use stale cache key %s, err: %v\n", key, err)
			return item.Object, nil
		}
		return nil, err
	}

	enveloper.locker.Lock()
	defer enveloper.locker.Unlock()

	var expiredAt time.Time
	if enveloper.expiration > 0 {
		expiredAt = time.Now().Add(enveloper.expiration)
	}
	enveloper.items[key] = Item{
		Object:    decryptedKey,
		ExpiredAt: expiredAt,
	}
	return decryptedKey, nil
}

func (enveloper *Enveloper) SerializeEncryptedKey(encryptedKey []byte) []byte {
	return enveloper.wrapped.SerializeEncryptedKey(encryptedKey)
}

func EnveloperWithCache(enveloper envelope.Enveloper, expiration time.Duration) envelope.Enveloper {
	return &Enveloper{
		wrapped:    enveloper,
		items:      make(map[string]Item),
		expiration: expiration,
	}
}
