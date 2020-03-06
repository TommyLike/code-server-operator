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
	"strings"

	"time"

	csv1alpha1 "github.com/tommylike/code-server-operator/api/v1alpha1"
)

const TimeLayout = "2006-01-02T15:04:05.000Z"

// CodeServerWatcher watches all living code server
type CodeServerWatcher struct {
	client.Client
	Log           logr.Logger
	Scheme        *runtime.Scheme
	Options       *CodeServerOption
	reqCh         <-chan CodeServerRequest
	probeCh       <-chan time.Time
	inActiveCache *CodeServerActiveCache
	recyclCache   *CodeServerRecycleCache
}

func (cs *CodeServerWatcher) inActiveCodeServer(req types.NamespacedName) {
	reqLogger := cs.Log.WithValues("codeserverwatcher", req)
	codeServer := &csv1alpha1.CodeServer{}
	err := cs.Client.Get(context.TODO(), req, codeServer)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("CodeServer has been deleted. Ignore inactive code server.")
			return
		}
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

func (cs *CodeServerWatcher) recycleCodeServer(req types.NamespacedName) {
	reqLogger := cs.Log.WithValues("codeserverwatcher", req)
	codeServer := &csv1alpha1.CodeServer{}
	err := cs.Client.Get(context.TODO(), req, codeServer)
	if err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("CodeServer has been deleted. Ignore recycle code server.")
			return
		}
	}
	if !HasCondition(codeServer.Status, csv1alpha1.ServerRecycled) {
		recycleCondition := NewStateCondition(csv1alpha1.ServerRecycled,
			"code server has been marked recycled", "")
		SetCondition(&codeServer.Status, recycleCondition)
		err := cs.Client.Update(context.TODO(), codeServer)
		if err != nil {
			reqLogger.Error(err, "Failed to update code server status.")
		}
	}
}

func NewCodeServerWatcher(client client.Client, log logr.Logger, schema *runtime.Scheme,
	options *CodeServerOption, reqCh <-chan CodeServerRequest, probeCh <-chan time.Time) *CodeServerWatcher {
	cache := CodeServerActiveCache{}
	cache.InactiveCaches = make(map[string]CodeServerActiveStatus)
	recycleCache := CodeServerRecycleCache{}
	recycleCache.Caches = make(map[string]CodeServerRecycleStatus)
	return &CodeServerWatcher{
		client,
		log,
		schema,
		options,
		reqCh,
		probeCh,
		&cache,
		&recycleCache,
	}
}

func (cs *CodeServerWatcher) Run(stopCh <-chan struct{}) {
	for {
		select {
		case event := <-cs.reqCh:
			switch event.operate {
			case AddInactiveWatch:
				cs.inActiveCache.AddOrUpdate(event)
			case DeleteInactiveWatch:
				cs.inActiveCache.Delete(event)
			case AddRecycleWatch:
				cs.recyclCache.AddOrUpdate(event)
			case DeleteRecycleWatch:
				cs.recyclCache.Delete(event)
			}
		case <-cs.probeCh:
			cs.ProbeAllCodeServer()
			cs.ProbeAllInactivedCodeServer()
		case <-stopCh:
			return
		}
	}
}

func (cs *CodeServerWatcher) ProbeAllInactivedCodeServer() {
	reqLogger := cs.Log.WithName("codeserverwatcher")
	for _, key := range cs.recyclCache.GetKeys() {
		css := cs.recyclCache.Get(key)
		if css != nil {
			reqLogger.Info(fmt.Sprintf("starting to determine whether inactive code server %s should be deleted", key))
			if cs.CodeServerNowShouldDelete(css.LastInactiveTime.Time, css.NamespacedName.String(), css.Duration) {
				cs.recycleCodeServer(css.NamespacedName)
				cs.recyclCache.DeleteFromName(css.NamespacedName)
			}
		}
	}
}

func (cs *CodeServerWatcher) ProbeAllCodeServer() {
	reqLogger := cs.Log.WithName("codeserverwatcher")
	for _, key := range cs.inActiveCache.GetKeys() {
		css := cs.inActiveCache.Get(key)
		if css != nil {
			reqLogger.Info(fmt.Sprintf("starting to probe code server endpoint %s", key))
			valid, t := cs.ProbeCodeServer(key, css)
			if !valid {
				if css.FailureCount > cs.Options.MaxProbeRetry {
					reqLogger.Info(fmt.Sprintf("probe code server %s failed and exceed max retries", key))
					cs.inActiveCodeServer(css.NamespacedName)
					cs.inActiveCache.DeleteFromName(css.NamespacedName)
				} else {
					cs.inActiveCache.BumpFailureCount(key)
				}
			} else {
				if cs.CodeServerNowInactive(*t, key, css.Duration) {
					cs.inActiveCodeServer(css.NamespacedName)
					cs.inActiveCache.DeleteFromName(css.NamespacedName)
				}
			}
		}
	}
}

func (cs *CodeServerWatcher) ProbeCodeServer(key string, css *CodeServerActiveStatus) (bool, *time.Time) {
	reqLogger := cs.Log.WithValues("codeserverwatcher", key)
	resp, err := http.Get(css.ProbeEndpoint)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("failed to probe the codeserver %s", key))
		return false, nil
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("failed to parse body from probe endpoint %s", css.ProbeEndpoint))
		return false, nil
	}

	if resp.StatusCode == 204 {
		reqLogger.Info(fmt.Sprintf("not content found in code server %s 's mtime attribute, maybe's been created now.", key))
		return false, nil
	}
	if resp.StatusCode != 200 {
		reqLogger.Error(err, fmt.Sprintf("failed to parse body from probe endpoint , status code %d", resp.StatusCode))
		return false, nil
	}
	timeStr := strings.Trim(string(body), "\"")
	reqLogger.Info(fmt.Sprintf("probe liveness time %s for code server %s", timeStr, key))
	t, err := time.Parse(TimeLayout, timeStr)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("failed to parse time string into time format %s", timeStr))
		return false, nil
	}
	return true, &t
}

func (cs *CodeServerWatcher) CodeServerNowInactive(mtime time.Time, key string, duration int64) bool {
	reqLogger := cs.Log.WithName("codeserverwatcher")
	elapsed := time.Now().Sub(mtime)
	if elapsed.Seconds() > float64(duration) {
		reqLogger.Info(fmt.Sprintf("code server %s probed long time invalid, last active time %s and now %s", key, mtime, time.Now()))
		return true
	}
	return false
}

func (cs *CodeServerWatcher) CodeServerNowShouldDelete(mtime time.Time, key string, duration int64) bool {
	reqLogger := cs.Log.WithName("codeserverwatcher")
	elapsed := time.Now().Sub(mtime)
	if elapsed.Seconds() > float64(duration) {
		reqLogger.Info(fmt.Sprintf("code server %s now should be deleted due to recycle time exceeds", key))
		return true
	}
	return false
}
