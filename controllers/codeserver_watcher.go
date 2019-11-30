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
	"context"
	"fmt"
	"github.com/go-logr/logr"
	"io/ioutil"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sync"
	"time"

	csv1alpha1 "github.com/tommylike/code-server-operator/api/v1alpha1"
)

type CodeServerCache struct {
	sync.RWMutex
	Caches map[string]CodeServerStatus
}

type CodeServerStatus struct {
	ProbeEndpoint string
	Duration int64
	FailureCount int
	NamespacedName types.NamespacedName
}

func (c *CodeServerCache)AddOrUpdate(req CodeServerRequest) {
	c.Lock()
	defer c.Unlock()
	if obj, found := c.Caches[req.resource.String()]; found {
		obj.Duration = req.duration
		obj.ProbeEndpoint = req.endpoint
	} else {
		c.Caches[req.resource.String()] = CodeServerStatus{
			ProbeEndpoint: req.endpoint,
			Duration: req.duration,
			FailureCount:0,
			NamespacedName: req.resource,
		}
	}
}

func (c *CodeServerCache)Delete(req CodeServerRequest) {
	c.Lock()
	defer c.Unlock()
	if _, found := c.Caches[req.resource.String()]; found {
		delete(c.Caches, req.resource.String())
	}
}

func (c *CodeServerCache)DeleteFromName(req types.NamespacedName) {
	c.Lock()
	defer c.Unlock()
	if _, found := c.Caches[req.String()]; found {
		delete(c.Caches, req.String())
	}
}

func (c *CodeServerCache)BumpFailureCount(key string) {
	c.Lock()
	defer c.Unlock()
	if obj, found := c.Caches[key]; found {
		obj.FailureCount += 1
	}
}
func (c *CodeServerCache)Get(key string) *CodeServerStatus{
	c.RLock()
	defer c.RUnlock()
	if obj, found := c.Caches[key]; found {
		return &obj
	}
	return nil
}

func (c *CodeServerCache)GetKeys() []string {
	var result []string
	c.RLock()
	defer c.RUnlock()
	for k := range c.Caches {
		result = append(result, k)
	}
	return result

}


// CodeServerWatcher watches all living code server
type CodeServerWatcher struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
	Options *CodeServerOption
	reqCh  <-chan CodeServerRequest
	probeCh <-chan time.Time
	cache *CodeServerCache
}

func (cs *CodeServerWatcher)inActiveCodeServer(req types.NamespacedName) {
	reqLogger := cs.Log.WithValues("codeserverwatcher", req)
	codeServer := &csv1alpha1.CodeServer{}
	err := cs.Client.Get(context.TODO(), req, codeServer)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("CodeServer has been deleted. Ignore inactive code server.")
			return
		}
		if !HasCondition(codeServer.Status, csv1alpha1.ServerInactive) && !HasCondition(codeServer.Status, csv1alpha1.ServerRecycled) {
			inactiveCondition := NewStateCondition(csv1alpha1.ServerInactive,
				"code server has been marked inactive", "")
			SetCondition(&codeServer.Status, inactiveCondition)
			err := cs.Client.Update(context.TODO(), codeServer)
			if err != nil {
				reqLogger.Error(err, "Failed to update code server status.")
			}
		}
	}
}

func NewCodeServerWatcher(client client.Client, log logr.Logger, schema *runtime.Scheme,
	options *CodeServerOption, reqCh <-chan CodeServerRequest, probeCh <-chan time.Time) *CodeServerWatcher {
	cache := CodeServerCache{}
	cache.Caches = make(map[string]CodeServerStatus)
	return &CodeServerWatcher{
		client,
		log,
		schema,
		options,
		reqCh,
		probeCh,
		&cache,
	}
}

func (cs *CodeServerWatcher)Run(stopCh <-chan struct{}){
	for {
		select {
		case event := <-cs.reqCh:
			switch event.operate {
			case AddWatch:
				cs.cache.AddOrUpdate(event)
			case DeleteWatch:
				cs.cache.Delete(event)
			}
		case <-cs.probeCh:
			cs.ProbeAllCodeServer()
		case <-stopCh:
			return
		}
	}
}

func (cs *CodeServerWatcher) ProbeAllCodeServer() {
	reqLogger := cs.Log.WithName("codeserverwatcher")
	for _, key := range cs.cache.GetKeys() {
		css := cs.cache.Get(key)
		if css != nil {
			reqLogger.Info(fmt.Sprintf("starting to probe code server endpoint %s", key))
			valid, t := cs.ProbeCodeServer(key, css)
			if !valid {
				if css.FailureCount > cs.Options.MaxProbeRetry {
					reqLogger.Info(fmt.Sprintf("probe code server %s failed and exceed max retries", key))
					cs.inActiveCodeServer(css.NamespacedName)
					cs.cache.DeleteFromName(css.NamespacedName)
				} else {
					cs.cache.BumpFailureCount(key)
				}
			} else {
				if cs.CodeServerNowInactive(*t, key, css.Duration) {
					cs.inActiveCodeServer(css.NamespacedName)
					cs.cache.DeleteFromName(css.NamespacedName)
				}
			}
		}
	}
}

func (cs *CodeServerWatcher)ProbeCodeServer(key string, css *CodeServerStatus) (bool, *time.Time) {
	reqLogger := cs.Log.WithValues("codeserverwatcher", key)
	resp , err := http.Get(css.ProbeEndpoint)
	if err != nil {
		reqLogger.Error(err,fmt.Sprintf("failed to probe the codeserver %s", key))
		return false, nil
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		reqLogger.Error(err,fmt.Sprintf("failed to parse body from probe endpoint %s", css.ProbeEndpoint))
		return false, nil
	}
	if resp.StatusCode != 200 {
		reqLogger.Error(err,fmt.Sprintf("failed to parse body from probe endpoint , status code %d", resp.StatusCode))
		return false, nil
	}
	reqLogger.Info("probe liveness time %s for code server %s", string(body), key)
	layout := "2006-01-02T15:04:05.000Z"
	t, err := time.Parse(layout, string(body))
	if err != nil {
		reqLogger.Error(err,fmt.Sprintf("failed to parse time string into time format %s", string(body)))
	}
	return true, &t
}

func (cs *CodeServerWatcher)CodeServerNowInactive(mtime time.Time, key string, duration int64) bool {
	elapsed := time.Now().Sub(mtime)
	if elapsed.Seconds() > float64(duration) {
		return true
	}
	return false
}
