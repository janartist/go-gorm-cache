package gorm_cache

import (
	"context"
	"fmt"
	"gorm.io/gorm"
	"log"
	"strconv"
	"time"
)

const (
	pluginName     = "cache"
	primaryValJoin = "_"
	confDBKey      = "cache_conf"
)

var (
	DefaultConf = &Conf{
		Prefix:      "gorm_cache_",
		IsReadSet:   true,
		IsCreateSet: false,
		Ttl:         0,
	}
	errKeyNotFound = fmt.Errorf("key not found")
)

type Conf struct {
	Prefix string `json:"prefix" yaml:"prefix"`
	// 开启读写
	IsReadSet bool `json:"is_read_set" yaml:"is_read_set"`
	// 开启双写
	IsCreateSet bool `json:"is_create_set" yaml:"is_create_set"`
	// 默认过期时间
	Ttl time.Duration `json:"ttl" yaml:"ttl"`
}
type DB struct {
	Conf  *Conf
	Store StoreInterface
}

type StoreInterface interface {
	Del(context.Context, string) error
	Set(context.Context, string, interface{}, time.Duration) error
	Get(context.Context, string, interface{}) error
}

type ConfigInterface interface {
	GetCacheConf() *Conf
}

type KeyInterface interface {
	GetCacheKey() string
}

type EnableInterface interface {
	IsCacheEnable() bool
}

type DisableInterface interface {
	IsCacheDisable() bool
}

func NewDBCache(client StoreInterface, conf *Conf) *DB {
	return &DB{Store: client, Conf: conf}
}

func (d *DB) Name() string {
	return pluginName
}

func (d *DB) Initialize(db *gorm.DB) error {
	db.Callback().Query().Before("gorm:before_query").Register(pluginName+":before_read", d.Before)
	db.Callback().Create().Before("gorm:before_create").Register(pluginName+":before_create", d.Before)
	db.Callback().Update().Before("gorm:before_update").Register(pluginName+":before_create", d.Before)
	db.Callback().Delete().Before("gorm:before_delete").Register(pluginName+":before_create", d.Before)

	db.Callback().Query().After("gorm:after_query").Register(pluginName+":after_read", d.AfterRead)

	db.Callback().Create().After("gorm:after_create").Register(pluginName+":after_create", d.AfterWrite)
	db.Callback().Update().After("gorm:after_update").Register(pluginName+":after_create", d.AfterWrite)
	db.Callback().Delete().After("gorm:after_delete").Register(pluginName+":after_create", d.AfterDelete)

	return nil
}

func (d *DB) Before(db *gorm.DB) {
	d.setConfig(db)
}

// 读后
func (d *DB) AfterRead(db *gorm.DB) {
	if !d.IsEnable(db) {
		return
	}
	conf := d.getConfig(db)
	if conf.IsReadSet {
		err := d.setCache(db, false)
		if err != nil {
			log.Print("set cache error:", err)
		}
	}
}

// 增删改后
func (d *DB) AfterWrite(db *gorm.DB) {
	if !d.IsEnable(db) {
		return
	}
	conf := d.getConfig(db)
	if conf.IsCreateSet {
		err := d.setCache(db, false)
		if err != nil {
			log.Print("set cache error:", err)
		}
	} else {
		err := d.delCache(db)
		if err != nil {
			log.Print("del cache error:", err)
		}
	}
}

func (d *DB) AfterDelete(db *gorm.DB) {
	if !d.IsEnable(db) {
		return
	}
	conf := d.getConfig(db)
	if conf.IsCreateSet {
		err := d.setCache(db, true)
		if err != nil {
			log.Print("set cache error:", err)
		}
	} else {
		err := d.delCache(db)
		if err != nil {
			log.Print("del cache error:", err)
		}
	}
}

func (d *DB) GetFromCache(db *gorm.DB, table string, key interface{}, val interface{}) error {
	cacheKey := d.Conf.Prefix + table + toStr(key)
	return d.Store.Get(db.Statement.Context, cacheKey, val)
}

func (d *DB) Remember(db *gorm.DB) {

}

func (d *DB) setCache(db *gorm.DB, isNil bool) error {
	key, err := d.getCacheKey(db)
	conf := d.getConfig(db)
	if err == nil {
		val := db.Statement.Model
		if isNil {
			val = "{}"
		}
		return d.Store.Set(db.Statement.Context, key, val, conf.Ttl)
	}
	return err
}

func (d *DB) delCache(db *gorm.DB) error {
	key, err := d.getCacheKey(db)
	if err == nil {
		return d.Store.Del(db.Statement.Context, key)
	}
	return err
}

// 获取key
func (d *DB) getCacheKey(db *gorm.DB) (string, error) {
	key, err := d.getKey(db)
	if err != nil {
		return "", err
	}
	return d.Conf.Prefix + db.Statement.Table + key, nil
}

// 获取key
func (d *DB) getKey(db *gorm.DB) (string, error) {
	var key = d.getPrimaryVal(db)
	if keyInterface, ok := db.Statement.Model.(KeyInterface); ok {
		key = keyInterface.GetCacheKey()
	}
	if key == "" {
		return "", errKeyNotFound
	}
	return key, nil
}

// 设置config
func (d *DB) setConfig(db *gorm.DB) {
	var conf = d.Conf
	if configInterface, ok := db.Statement.Model.(ConfigInterface); ok {
		conf = configInterface.GetCacheConf()
	}
	db.Set(confDBKey, conf)
}

// 获取conf
func (d *DB) getConfig(db *gorm.DB) *Conf {
	if val, ok := db.Get(confDBKey); ok {
		return val.(*Conf)
	}
	return DefaultConf
}

func (d *DB) getPrimaryVal(db *gorm.DB) string {
	var primaryVal string
	for _, field := range db.Statement.Schema.PrimaryFields {
		if fieldValue, isZero := field.ValueOf(db.Statement.Context, db.Statement.ReflectValue); !isZero {
			primaryVal += toStr(fieldValue)
		}
	}
	return primaryVal
}

func toStr(fieldValue interface{}) string {
	value := ""
	switch val := fieldValue.(type) {
	case int:
		value = strconv.Itoa(val)
	case int8:
		value = strconv.FormatInt(int64(val), 10)
	case int16:
		value = strconv.FormatInt(int64(val), 10)
	case int32:
		value = strconv.FormatInt(int64(val), 10)
	case int64:
		value = strconv.FormatInt(val, 10)
	case uint:
		value = strconv.FormatUint(uint64(val), 10)
	case uint8:
		value = strconv.FormatUint(uint64(val), 10)
	case uint16:
		value = strconv.FormatUint(uint64(val), 10)
	case uint32:
		value = strconv.FormatUint(uint64(val), 10)
	case uint64:
		value = strconv.FormatUint(val, 10)
	case string:
		value = val
	}
	return value
}

func (d *DB) IsEnable(db *gorm.DB) bool {
	if enableInterface, ok := db.Statement.Model.(EnableInterface); ok {
		return enableInterface.IsCacheEnable()
	}
	if disableInterface, ok := db.Statement.Model.(DisableInterface); ok {
		return !disableInterface.IsCacheDisable()
	}
	return db.RowsAffected > 0
}
