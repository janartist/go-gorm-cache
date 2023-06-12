package store

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"time"
)

type memory struct {
	sync.RWMutex
	items map[string]item
}

type item struct {
	value      interface{}
	expireTime time.Time
	isForever  bool
}

func NewMemory() *memory {
	return &memory{items: make(map[string]item)}
}

func (m *memory) Del(ctx context.Context, key string) error {
	m.Lock()
	delete(m.items, key)
	m.Unlock()
	return nil
}

func (m *memory) Get(ctx context.Context, key string, val interface{}) error {
	m.Lock()
	defer func() {
		m.Unlock()
	}()
	if i, ok := m.items[key]; ok {
		if i.isExpired() {
			delete(m.items, key)
		}
		if !i.isExpired() && reflect.ValueOf(val).Kind() == reflect.Ptr {
			reflect.ValueOf(val).Elem().Set(reflect.Indirect(reflect.ValueOf(i.value)))
			return nil
		}
		return errors.New("val is not Ptr")

	}
	return errors.New("val is nil")
}

func (m *memory) Set(ctx context.Context, key string, val interface{}, ttl time.Duration) error {
	isForever := false
	expireTime := time.Now().Add(ttl)
	if ttl < 0 {
		isForever = true
	}
	m.Lock()
	m.items[key] = item{
		value:      val,
		expireTime: expireTime,
		isForever:  isForever,
	}
	m.Unlock()
	return nil
}

// isExpired 判断对象是否过期
func (i *item) isExpired() bool {
	if i.isForever {
		return false
	}
	return i.expireTime.Unix() <= time.Now().Unix()
}
