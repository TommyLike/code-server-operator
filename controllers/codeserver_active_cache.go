/*
Copyright 2019 tommylikehu@gmail.com.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"k8s.io/apimachinery/pkg/types"
	"sync"
)

type CodeServerActiveCache struct {
	sync.RWMutex
	InactiveCaches map[string]CodeServerActiveStatus
}

type CodeServerActiveStatus struct {
	ProbeEndpoint  string
	Duration       int64
	FailureCount   int
	NamespacedName types.NamespacedName
}

func (c *CodeServerActiveCache) AddOrUpdate(req CodeServerRequest) {
	c.Lock()
	defer c.Unlock()
	if obj, found := c.InactiveCaches[req.resource.String()]; found {
		obj.Duration = req.duration
		obj.ProbeEndpoint = req.endpoint
	} else {
		c.InactiveCaches[req.resource.String()] = CodeServerActiveStatus{
			ProbeEndpoint:  req.endpoint,
			Duration:       req.duration,
			FailureCount:   0,
			NamespacedName: req.resource,
		}
	}
}

func (c *CodeServerActiveCache) Delete(req CodeServerRequest) {
	c.Lock()
	defer c.Unlock()
	if _, found := c.InactiveCaches[req.resource.String()]; found {
		delete(c.InactiveCaches, req.resource.String())
	}
}

func (c *CodeServerActiveCache) DeleteFromName(req types.NamespacedName) {
	c.Lock()
	defer c.Unlock()
	if _, found := c.InactiveCaches[req.String()]; found {
		delete(c.InactiveCaches, req.String())
	}
}

func (c *CodeServerActiveCache) BumpFailureCount(key string) {
	c.Lock()
	defer c.Unlock()
	if obj, found := c.InactiveCaches[key]; found {
		obj.FailureCount += 1
	}
}
func (c *CodeServerActiveCache) Get(key string) *CodeServerActiveStatus {
	c.RLock()
	defer c.RUnlock()
	if obj, found := c.InactiveCaches[key]; found {
		return &obj
	}
	return nil
}

func (c *CodeServerActiveCache) GetKeys() []string {
	var result []string
	c.RLock()
	defer c.RUnlock()
	for k := range c.InactiveCaches {
		result = append(result, k)
	}
	return result

}
