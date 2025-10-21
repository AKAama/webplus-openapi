package store

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/dgraph-io/badger/v4"
	"github.com/timshannon/badgerhold/v4"
	"go.uber.org/zap"
)

var instance *BadgerStore
var once sync.Once

type BadgerStore struct {
	store *badgerhold.Store
}

func (b BadgerStore) Store(key string, value interface{}) error {
	return b.store.Upsert(key, value)
}

func (b BadgerStore) Get(key string, value interface{}) error {
	return b.store.Get(key, value)
}

func (b BadgerStore) Delete(key string) error {
	return b.store.Delete(key, nil)
}

func (b BadgerStore) Exists(key string) bool {
	var result interface{}
	err := b.store.Get(key, &result)
	return err == nil
}

func (b BadgerStore) View(fn func(txn *badger.Txn) error) error {
	return b.store.Badger().View(fn)
}

func (b BadgerStore) Upsert(key string, value interface{}) error {
	return b.store.Upsert(key, value)
}

func (b BadgerStore) DeleteMatching(value interface{}, query badgerhold.Query) error {
	return b.store.DeleteMatching(value, &query)
}

// BadgerStoreAdapter 适配器结构体
type BadgerStoreAdapter struct {
	store *BadgerStore
}

// NewBadgerStoreAdapter 创建适配器实例
func NewBadgerStoreAdapter(badgerStore *BadgerStore) *BadgerStoreAdapter {
	return &BadgerStoreAdapter{
		store: badgerStore,
	}
}

func (a *BadgerStoreAdapter) Store(key string, value interface{}) error {
	return a.store.Store(key, value)
}

func (a *BadgerStoreAdapter) Get(key string, value interface{}) error {
	return a.store.Get(key, value)
}

func (a *BadgerStoreAdapter) Delete(key string) error {
	return a.store.Delete(key)
}

func (a *BadgerStoreAdapter) Exists(key string) bool {
	return a.store.Exists(key)
}

func (a *BadgerStoreAdapter) View(fn func(txn *badger.Txn) error) error {
	return a.store.View(fn)
}

func (a *BadgerStoreAdapter) Upsert(key string, value interface{}) error {
	return a.store.Upsert(key, value)
}

func (a *BadgerStoreAdapter) DeleteMatching(value interface{}, query badgerhold.Query) error {
	return a.store.DeleteMatching(value, query)
}

func GetBadgerStore() *BadgerStore {
	once.Do(func() {
		p, err := os.Getwd()
		if err != nil {
			panic(err)
		}
		options := badgerhold.DefaultOptions
		filePath := filepath.Join(p, "etc", "data")
		options.Dir = filePath
		options.ValueDir = filePath
		store, err := badgerhold.Open(options)
		if err != nil {
			panic(err)
		}
		instance = &BadgerStore{store: store}
	})
	return instance
}

func CloseBadgerStore() {
	if instance != nil {
		zap.S().Info("正在关闭 Badger 存储...")
		err := instance.store.Close()
		if err != nil {
			zap.S().Errorf("关闭 Badger 存储时发生错误: %v", err)
		} else {
			zap.S().Info("Badger 存储已成功关闭")
		}
		// 重置实例，避免重复关闭
		instance = nil
	}
}
