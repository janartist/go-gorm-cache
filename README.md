## gorm cache plugin

### 用于gorm的缓存插件

- 0侵入业务代码
- 通过gorm callback实现 gorm:update及gorm:delete 后 cache del以及cache set
- 通过gorm callback实现 gorm:query 前 cache get
- 实现gorm:query时防缓存击穿
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
		EnableReadSet:  true,
		Ttl:            time.Minute * 10,
	}
}

// 启用(优先级比禁用高)
func (p Product) IsCacheEnable() bool {
	return true
}

// 禁用
func (p Product) IsCacheDisable() bool {
	return false
}

func (p Product) MarshalBinary() (data []byte, err error) {
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
	db.Create(&Product{Code: "Create", Price: 100})
	// Read
	var product Product
	db.First(&product, 1) // 根据整型主键查找
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

```

### Tips

- 配置设置请勿使用指针方法，以免模型断言成接口会失败,因为gorm中的模型可以为指针或结构体
```go
func (p Product) GetCacheConf() go_gorm_cache.Conf {
	return go_gorm_cache.Conf{
		EnableWriteSet: true,
		EnableReadSet:  true,
		Ttl:            time.Minute * 10,
	}
}
// 启用(优先级比禁用高)
func (p Product) IsCacheEnable() bool {
    return true
}

// 禁用
func (p Product) IsCacheDisable() bool {
    return false
}
```
- 增删查改需要统一使用一个模型（结构体），勿切换其他以免出现主键未找到等情况。且主键需用 ```gorm:"primarykey"```标签标明