/*
Copyright 2020 Gravitational, Inc.

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

package workpool

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gopkg.in/check.v1"
)

func Example() {
	pool := NewPool(context.TODO())
	defer pool.Stop()
	// single key which wants 3 concurrent workers.
	pool.Set("my-key", 3)
	var wg sync.WaitGroup
	// spawn 6 workers which have 50ms of work to do each.
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			lease := <-pool.Acquire()
			defer lease.Release()
			if k := lease.Key().(string); k != "my-key" {
				panic("unexpected key: " + k)
			}
			time.Sleep(30 * time.Millisecond)
			wg.Done()
		}()
	}
	start := time.Now()
	wg.Wait()
	fmt.Printf("%s", time.Since(start).Round(time.Millisecond*10)) // Output: 60ms
}

func Test(t *testing.T) {
	check.TestingT(t)
}

type WorkSuite struct{}

var _ = check.Suite(&WorkSuite{})

func (s *WorkSuite) TestPool(c *check.C) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	p := NewPool(ctx)
	key := "some-key"
	p.Set(key, 1)
	for i := 0; i < 100; i++ {
		select {
		case l := <-p.Acquire():
			c.Assert(l.Key().(string), check.Equals, key)
			l.Release()
		case <-time.After(time.Millisecond * 128):
			c.Errorf("timeout waiting for lease grant")
		}
	}
	p.Set(key, 0)
	time.Sleep(time.Millisecond * 32)
	select {
	case l := <-p.Acquire():
		c.Errorf("unexpected lease grant: %+v", l)
	case <-time.After(time.Millisecond * 128):
	}
	var wg sync.WaitGroup
	wgdone := make(chan struct{})
	go func() {
		wg.Wait()
		close(wgdone)
	}()
	g1done := make(chan struct{})
	p.Set(key, 200)
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			select {
			case l := <-p.Acquire():
				<-g1done
				l.Release()
			case <-time.After(time.Millisecond * 512):
				c.Errorf("Timeout waiting for lease")
			}
			wg.Done()
		}()
	}
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			select {
			case <-p.Acquire():
				// leak deliberately
			case <-time.After(time.Millisecond * 512):
				c.Errorf("Timeout waiting for lease")
			}
			wg.Done()
		}()
	}
	close(g1done)
	wg.Wait()
	select {
	case l := <-p.Acquire():
		counts := l.(*lease).loadCounts()
		c.Errorf("unexpected lease grant: %+v, counts=%+v", l, counts)
	case <-time.After(time.Millisecond * 128):
	}
	p.Set(key, 201)
	select {
	case l := <-p.Acquire():
		c.Assert(l.Key().(string), check.Equals, key)
		l.Release()
	case <-time.After(time.Millisecond * 128):
		c.Errorf("timeout waiting for lease grant")
	}
}
