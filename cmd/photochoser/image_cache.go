package main

import (
	stdlist "container/list"
	"image"
	"sync"
)

type imageCacheEntry struct {
	path string
	img  image.Image
}

type imageCache struct {
	mu      sync.Mutex
	limit   int
	entries map[string]*stdlist.Element
	order   *stdlist.List
}

func newImageCache(limit int) *imageCache {
	if limit < 1 {
		limit = 1
	}
	return &imageCache{
		limit:   limit,
		entries: make(map[string]*stdlist.Element),
		order:   stdlist.New(),
	}
}

func (c *imageCache) Load(path string) (image.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.entries[path]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	return elem.Value.(*imageCacheEntry).img, true
}

func (c *imageCache) Store(path string, img image.Image) bool {
	if img == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.entries[path]; ok {
		elem.Value.(*imageCacheEntry).img = img
		c.order.MoveToFront(elem)
		return false
	}

	c.entries[path] = c.order.PushFront(&imageCacheEntry{path: path, img: img})
	evicted := false
	for len(c.entries) > c.limit {
		c.removeOldestLocked()
		evicted = true
	}
	return evicted
}

func (c *imageCache) DeleteExcept(keep map[string]struct{}) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	deleted := false
	for path, elem := range c.entries {
		if _, ok := keep[path]; ok {
			continue
		}
		c.removeElementLocked(elem)
		deleted = true
	}
	return deleted
}

func (c *imageCache) Clear() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) == 0 {
		return false
	}
	c.entries = make(map[string]*stdlist.Element)
	c.order.Init()
	return true
}

func (c *imageCache) removeOldestLocked() {
	if elem := c.order.Back(); elem != nil {
		c.removeElementLocked(elem)
	}
}

func (c *imageCache) removeElementLocked(elem *stdlist.Element) {
	entry := elem.Value.(*imageCacheEntry)
	delete(c.entries, entry.path)
	c.order.Remove(elem)
}
