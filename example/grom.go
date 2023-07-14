package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	go_gorm_cache "github.com/janartist/go-gorm-cache"
	"github.com/janartist/go-gorm-cache/store"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type Product struct {
	gorm.Model
	Code  string
	Price uint
}

// 自定义配置
func (p Product) GetCacheConf() go_gorm_cache.Conf {
	return go_gorm_cache.Conf{
		EnableWriteSet: true,
		EnableFindSet:  true,
		Ttl:            time.Minute * 10,
	}
}

// 启用(优先级比禁用高)
func (p *Product) IsCacheEnable() bool {
	return true
}

// 禁用
func (p *Product) IsCacheDisable() bool {
	return false
}

func (p *Product) MarshalBinary() (data []byte, err error) {
	return json.Marshal(p)
}

func (p *Product) UnmarshalBinary(data []byte) error {
	return json.Unmarshal(data, p)
}

func main() {
	db, err := gorm.Open(sqlite.Open("file:mockdb?mode=memory&cache=shared&_auto_vacuum=none"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}

	//opt, err := redis.ParseURL(fmt.Sprintf("redis://:%s@%s/%d", "", "10.1.2.7:6379", 3))
	//if err != nil {
	//	panic(err)
	//}
	//rd := store.NewRedis(redis.NewClient(opt))
	//
	//cache := go_gorm_cache.NewDBCache(rd, go_gorm_cache.DefaultConf)
	//err = db.Use(cache)
	cache := go_gorm_cache.NewDBCache(store.NewMemory(), go_gorm_cache.Conf{})
	err = db.Use(cache)
	if err != nil {
		panic("db.Use connect database")
	}
	db = db.Debug()

	// 迁移 schema
	db.AutoMigrate(&Product{})
	// Create
	db.Model(Product{}).Create(&Product{Code: "Create", Price: 100})
	// Read
	var product Product
	db.Model(Product{}).First(&product, 1) // 根据整型主键查找
	fmt.Print("Hit cache Skip Db:", product, "\n")
	//db.First(&product, "code = ?", "D42") // 查找 code 字段值为 D42 的记录

	// Update - 将 product 的 price 更新为 200
	db.Model(&product).Update("Code", "Update")
	fmt.Print("Set Cache", product, "\n")
	// Update - 更新多个字段
	//db.Model(&product).Updates(Product{Price: 200, Code: "F42"}) // 仅更新非零值字段
	//db.Model(&product).Updates(map[string]interface{}{"Price": 200, "Code": "F42"})
	var product2 Product
	cache.Get(context.Background(), "products", 1, &product2)
	fmt.Print("Get Cache", product2, "\n")
	// Delete - 删除 product
	db.Delete(&product, 1)
}
