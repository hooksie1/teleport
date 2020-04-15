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
	// create two keys with different target counts
	pool.Set("spam", 2)
	pool.Set("eggs", 1)
	// track how many workers are spawned for each key
	counts := make(map[string]int)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			lease := <-pool.Acquire()
			defer lease.Release()
			mu.Lock()
			counts[lease.Key().(string)]++
			mu.Unlock()
			// in order to demonstrate the differing spawn rates we need
			// work to take some time, otherwise pool will end up granting
			// leases in a "round robin" fashion.
			time.Sleep(time.Millisecond * 10)
			wg.Done()
		}()
	}
	wg.Wait()
	// exact counts will vary, but leases with key `spam`
	// will end up being generated approximately twice as
	// often as leases with key `eggs`.
	fmt.Println(counts["spam"] > counts["eggs"]) // Output: true
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
