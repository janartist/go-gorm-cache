package gorm_cache

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/callbacks"
	clause2 "gorm.io/gorm/clause"
)

const (
	pluginName     = "cache"
	primaryValJoin = ":"
	confDBKey      = "cache_conf"
)

var (
	DefaultConf = &Conf{
		Prefix:         "gorm_cache",
		EnableFindSet:  false,
		EnableWriteSet: false,
		Ttl:            0,
	}
	errKeyNotFound = fmt.Errorf("key not found")
	// 缓存命中
	CacheHit = fmt.Errorf("cache hit")
	// 缓存跳过
	CacheSkip = fmt.Errorf("cache skip")
)

type Conf struct {
	Prefix string `json:"prefix" yaml:"prefix"`
	// 开启读库写缓存
	EnableFindSet bool `json:"enable_find_set" yaml:"enable_find_set"`
	// 开启写库写缓存, false为删缓存
	EnableWriteSet bool `json:"enable_create_set" yaml:"enable_create_set"`
	// 默认过期时间
	Ttl time.Duration `json:"ttl" yaml:"ttl"`
}
type DB struct {
	Conf   Conf
	Store  StoreInterface
	single Group
}

type StoreInterface interface {
	Del(context.Context, string) error
	Set(context.Context, string, interface{}, time.Duration) error
	Get(context.Context, string, interface{}) error
}

type ConfigInterface interface {
	GetCacheConf() Conf
}

type EnableInterface interface {
	IsCacheEnable() bool
}

type DisableInterface interface {
	IsCacheDisable() bool
}

func NewDBCache(client StoreInterface, conf Conf) *DB {
	return &DB{Store: client, Conf: conf}
}

func (d *DB) Name() string {
	return pluginName
}

func (d *DB) Initialize(db *gorm.DB) error {
	// 配置注册
	db.Callback().Query().Before("gorm:query").Register(pluginName+":before_read", d.Before())
	db.Callback().Create().Before("gorm:before_create").Register(pluginName+":before_create", d.Before())
	db.Callback().Update().Before("gorm:before_update").Register(pluginName+":before_update", d.Before())
	db.Callback().Delete().Before("gorm:before_delete").Register(pluginName+":before_delete", d.Before())
	// 读替换
	db.Callback().Query().Replace("gorm:query", d.Query())
	// 写后
	db.Callback().Create().After("gorm:after_create").Register(pluginName+":after_create", d.AfterWrite(false))
	db.Callback().Update().After("gorm:after_update").Register(pluginName+":after_create", d.AfterWrite(false))
	db.Callback().Delete().After("gorm:after_delete").Register(pluginName+":after_create", d.AfterWrite(true))

	return nil
}

func (d *DB) Get(ctx context.Context, tableName string, id interface{}, dest interface{}) error {
	key := d.Conf.Prefix + primaryValJoin + tableName + primaryValJoin + toStr(id)
	return d.Store.Get(ctx, key, dest)
}

func (d *DB) Before() func(*gorm.DB) {
	return func(db *gorm.DB) {
		d.setConfig(db)
	}
}

func (d *DB) Query() func(*gorm.DB) {
	return func(db *gorm.DB) {
		if !d.IsEnable(db) {
			callbacks.Query(db)
			return
		}
		if conf := d.getConfig(db); !conf.EnableFindSet {
			callbacks.Query(db)
			return
		}
		key, err := d.parseKeyFromQuery(db)
		if err != nil {
			log.Print("Query parseKeyFromQuery error:", err)
			callbacks.Query(db)
			return
		}
		err = d.getCacheFromQuery(db, key)
		if db.Error == CacheHit {
			return
		}
		if err != nil {
			log.Print("Query getCacheFromQuery error:", err)
		}
		localCache := false
		val, err := d.single.Do(key, func() (interface{}, error) {
			callbacks.Query(db)
			if db.RowsAffected <= 0 {
				return db.Statement.Dest, nil
			}
			err := d.setCache(db, false)
			if err != nil {
				log.Print("set cache error:", err)
			}
			localCache = true
			return db.Statement.Dest, err
		})
		if !localCache {
			if err == nil {
				db.AddError(CacheHit)
			}
			db.Statement.Dest = val
		}
	}

}

// 增删改后
func (d *DB) AfterWrite(isDel bool) func(*gorm.DB) {
	return func(db *gorm.DB) {
		if !d.IsEnable(db) {
			return
		}
		conf := d.getConfig(db)
		if conf.EnableWriteSet {
			err := d.setCache(db, isDel)
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
}

func (d *DB) setCache(db *gorm.DB, isNil bool) error {
	key, err := d.getCacheKeyFromPrimaryID(db, "")
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

// 从主键获取cache
func (d *DB) getCacheFromQuery(db *gorm.DB, key string) error {
	err := d.Store.Get(db.Statement.Context, key, db.Statement.Dest)
	if err == nil {
		db.AddError(CacheHit)
	}
	return err
}

// 从query获取cache key
func (d *DB) parseKeyFromQuery(db *gorm.DB) (key string, err error) {
	var (
		isPrimaryQuery = false
		primaryVal     = make([]interface{}, 0, 1)
	)
	// 解析是否为主键查询，if 太多  请勿效仿
	for _, clause := range db.Statement.Clauses {
		if clause.Name == "WHERE" {
			if clauseWhere, ok := clause.Expression.(clause2.Where); ok {
				if len(clauseWhere.Exprs) == 1 {
					if clauseIn, ok := clauseWhere.Exprs[0].(clause2.IN); ok {
						if clauseColumn, ok := clauseIn.Column.(clause2.Column); ok {
							if clauseColumn.Table == clause2.PrimaryColumn.Table && clauseColumn.Name == clause2.PrimaryColumn.Name {
								isPrimaryQuery = true
								primaryVal = clauseIn.Values
							}
						}
					}
				}
			}
		}
	}
	// 单行主键查询
	if isPrimaryQuery && len(primaryVal) == 1 {
		key, err = d.getCacheKeyFromPrimaryID(db, toStr(primaryVal[0]))
		return
	}
	err = CacheSkip
	return
}

func (d *DB) delCache(db *gorm.DB) error {
	key, err := d.getCacheKeyFromPrimaryID(db, "")
	if err == nil {
		return d.Store.Del(db.Statement.Context, key)
	}
	return err
}

// 获取key
func (d *DB) getCacheKeyFromPrimaryID(db *gorm.DB, id string) (string, error) {
	if id == "" {
		id = d.getPrimaryVal(db)
	}
	if id == "" {
		return "", errKeyNotFound
	}

	tableName := db.Statement.Table
	if db.Statement.Schema != nil {
		tableName = db.Statement.Schema.Table
	}
	return d.Conf.Prefix + primaryValJoin + tableName + primaryValJoin + id, nil
}

// 设置conf
func (d *DB) setConfig(db *gorm.DB) {
	var conf = d.Conf
	if configInterface, ok := db.Statement.Model.(ConfigInterface); ok {
		conf = configInterface.GetCacheConf()
	}
	db.InstanceSet(confDBKey, conf)
}

// 获取conf
func (d *DB) getConfig(db *gorm.DB) Conf {
	if val, ok := db.InstanceGet(confDBKey); ok {
		return val.(Conf)
	}
	return *DefaultConf
}

func (d *DB) getPrimaryVal(db *gorm.DB) string {
	var primaryVal string
	for _, field := range db.Statement.Schema.PrimaryFields {
		if fieldValue, isZero := field.ValueOf(db.Statement.Context, db.Statement.ReflectValue); !isZero {
			primaryVal += primaryValJoin + toStr(fieldValue)
		}
	}
	return strings.TrimPrefix(primaryVal, primaryValJoin)
}

func (d *DB) IsEnable(db *gorm.DB) bool {
	if enableInterface, ok := db.Statement.Model.(EnableInterface); ok {
		return enableInterface.IsCacheEnable()
	}
	if disableInterface, ok := db.Statement.Model.(DisableInterface); ok {
		return !disableInterface.IsCacheDisable()
	}
	return true
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
