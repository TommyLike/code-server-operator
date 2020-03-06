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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sync"
)

type CodeServerRecycleCache struct {
	sync.RWMutex
	Caches map[string]CodeServerRecycleStatus
}

type CodeServerRecycleStatus struct {
	Duration         int64
	LastInactiveTime metav1.Time
	NamespacedName   types.NamespacedName
}

func (c *CodeServerRecycleCache) AddOrUpdate(req CodeServerRequest) {
	c.Lock()
	defer c.Unlock()
	if obj, found := c.Caches[req.resource.String()]; found {
		obj.Duration = req.duration
		obj.LastInactiveTime = req.inactiveTime
	} else {
		c.Caches[req.resource.String()] = CodeServerRecycleStatus{
			Duration:         req.duration,
			LastInactiveTime: req.inactiveTime,
			NamespacedName:   req.resource,
		}
	}
}

func (c *CodeServerRecycleCache) Delete(req CodeServerRequest) {
	c.Lock()
	defer c.Unlock()
	if _, found := c.Caches[req.resource.String()]; found {
		delete(c.Caches, req.resource.String())
	}
}

func (c *CodeServerRecycleCache) DeleteFromName(req types.NamespacedName) {
	c.Lock()
	defer c.Unlock()
	if _, found := c.Caches[req.String()]; found {
		delete(c.Caches, req.String())
	}
}

func (c *CodeServerRecycleCache) Get(key string) *CodeServerRecycleStatus {
	c.RLock()
	defer c.RUnlock()
	if obj, found := c.Caches[key]; found {
		return &obj
	}
	return nil
}

func (c *CodeServerRecycleCache) GetKeys() []string {
	var result []string
	c.RLock()
	defer c.RUnlock()
	for k := range c.Caches {
		result = append(result, k)
	}
	return result

}
