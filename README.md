~~## gorm cache plugin

### 用于gorm的缓存插件

- 0侵入业务代码
- 通过gorm callback实现cache delete以及cache write set
- 每个模型可以单独配置
- 默认主键实现key
- 实现内存以及redis驱动

### Quick start

```shell
go get github.com/janartist/go-gorm-cache
```


```go
package main

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
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
func (p *Product) GetCacheConf() *go_gorm_cache.Conf {
	return &go_gorm_cache.Conf{
		IsCreateSet: true,
		IsReadSet:   true,
		Ttl:         time.Minute * 10,
	}
}

// 自定义key 默认主键
//func (p *Product) GetCacheKey() string {
//	return p.Code
//}

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

	opt, err := redis.ParseURL(fmt.Sprintf("redis://:%s@%s/%d", "", "10.1.2.7:6379", 3))
	if err != nil {
		panic(err)
	}
	rd := store.NewRedis(redis.NewClient(opt))

	cache := go_gorm_cache.NewDBCache(rd, go_gorm_cache.DefaultConf)

	err = db.Use(cache)

	//err = db.Use(go_gorm_cache.NewDBCache(store.NewMemory(), &go_gorm_cache.Conf{}))
	//if err != nil {
	//	panic("db.Use connect database")
	//}
	db2 := db.Debug()

	// 迁移 schema
	db.AutoMigrate(&Product{})
	// Create
	if err = db2.Create(&Product{Code: "D42", Price: 100}).Error; err != nil {
		panic("failed to connect database")
	}

	// Read
	var product Product
	db2.First(&product, 1) // 根据整型主键查找
	//db.First(&product, "code = ?", "D42") // 查找 code 字段值为 D42 的记录

	// Update - 将 product 的 price 更新为 200
	db2.Model(&product).Update("Code", "S33")
	// Update - 更新多个字段
	//db.Model(&product).Updates(Product{Price: 200, Code: "F42"}) // 仅更新非零值字段
	//db.Model(&product).Updates(map[string]interface{}{"Price": 200, "Code": "F42"})

	// Delete - 删除 product
	// db2.Delete(&product, 1)
	var p Product
	err = cache.GetFromCache(db2, "products", 2, &p)

	fmt.Print(p, err, "\n")
}

```