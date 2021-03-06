package sorm

import (
	"container/list"
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

func (d *Dao) QueryCache(indexes ...interface{}) (ModelIfe, error) {
	key, err := d.buildKey(indexes...)
	if err != nil {
		return nil, err
	}
	return d.Session().daoModelCache.Get(key)
}

func (d *Dao) RemoveCache(indexes ...interface{}) {
	if key, err := d.buildKey(indexes...); err == nil {
		d.Session().daoModelCache.Del(key)
	}
}

func (d *Dao) SaveCache(model ModelIfe) {
	if key, err := d.buildKey(model.IndexValues()...); err == nil {
		d.Session().daoModelCache.Put(key, model)
	}
}

func (d *Dao) buildKey(indexes ...interface{}) (string, error) {
	var err error
	buildStr := strings.Builder{}
	buildStr.WriteString(d.tableName)
	for _, v := range indexes {
	assert:
		switch m := v.(type) {
		case int, int64, uint, uint64, int32, int16, int8, uint32, uint16, uint8:
			buildStr.WriteString(fmt.Sprintf("`%d", m))
		case float32:
			buildStr.WriteString("`")
			buildStr.WriteString(strconv.FormatFloat(float64(m), 'f', -1, 64))
		case float64:
			buildStr.WriteString("`")
			buildStr.WriteString(strconv.FormatFloat(m, 'f', -1, 64))
		case string:
			if m == "" {
				err = NewError(ModelRuntimeError, "the index key can not be empty string")
			} else {
				buildStr.WriteString("`")
				buildStr.WriteString(m)
			}
		case []byte:
			buildStr.WriteString("`")
			buildStr.Write(m)
		case driver.Valuer:
			if v, err = m.Value(); err == nil {
				goto assert
			}
			err = NewError(ModelRuntimeError, err.Error())
		default:
			err = NewError(ModelRuntimeError, "not support index key")
		}
		if err != nil {
			return "", err
		}
	}
	return buildStr.String(), nil
}

// 使用lru算法缓存model
// 只在当前session有效

type element struct {
	listElem *list.Element
	model    ModelIfe
	time     time.Time
}

type modelLruCache struct {
	elements map[string]*element
	list     *list.List
	capacity int // 容量
	used     int // 使用量
	locker   sync.Mutex
}

func newDaoLru(capacity int) *modelLruCache {
	return &modelLruCache{
		elements: make(map[string]*element),
		list:     list.New(),
		capacity: capacity,
		used:     0,
	}
}

func (lru *modelLruCache) resetCapacity(capacity int) {
	lru.locker.Lock()
	defer lru.locker.Unlock()

	lru.capacity = capacity
	lru.delListFrontElement()
}

func (lru *modelLruCache) Clear() {
	lru.locker.Lock()
	defer lru.locker.Unlock()
	lru.elements = make(map[string]*element)
	lru.list.Init()
	lru.used = 0
	lru.capacity = daoModelLruCacheCapacity
}

func (lru *modelLruCache) Get(key string) (ModelIfe, error) {
	lru.locker.Lock()
	defer lru.locker.Unlock()
	if lru.used > 0 {
		if element, ok := lru.elements[key]; ok {
			lru.list.MoveToBack(element.listElem)
			return element.model, nil
		}
	}
	return nil, ModelNotFoundError
}

func (lru *modelLruCache) Put(key string, model ModelIfe) {

	lru.locker.Lock()
	defer lru.locker.Unlock()

	if lru.capacity < 1 {
		return
	}

	if elem, ok := lru.elements[key]; ok {
		lru.elements[key] = &element{listElem: elem.listElem, model: model}
		lru.list.MoveToBack(elem.listElem)
		return
	}
	lru.addElement(key, model)
	lru.delListFrontElement()
}

func (lru *modelLruCache) Del(key string) {

	lru.locker.Lock()
	defer lru.locker.Unlock()

	if element, ok := lru.elements[key]; ok {
		lru.list.Remove(element.listElem)
		delete(lru.elements, key)
		lru.used--
	}
}

func (lru *modelLruCache) addElement(key string, model ModelIfe) {
	lru.used++
	listElem := lru.list.PushBack(key)
	lru.elements[key] = &element{listElem: listElem, model: model, time: time.Now()}
}

func (lru *modelLruCache) delListFrontElement() {
	for used := lru.used; used > lru.capacity; used-- {
		frontElem := lru.list.Front()
		key := frontElem.Value.(string)
		if element, ok := lru.elements[key]; ok {
			lru.list.Remove(element.listElem)
			delete(lru.elements, key)
			lru.used--
		}
	}
}
