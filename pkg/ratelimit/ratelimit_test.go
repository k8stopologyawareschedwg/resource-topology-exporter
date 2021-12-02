/*
Copyright 2021 The Kubernetes Authors.

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
package ratelimit

import (
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnlimitedRateLimit(t *testing.T) {

	Convey("Given a RateLimiter", t, func() {

		const numberOfIterations int = 10
		histeresis_us := 70 * time.Microsecond

		sourceCh := make(chan Event)
		sut := NewUnlimited(sourceCh)
		sut.Run()

		done := make(chan struct{})
		go sender(t, sourceCh, numberOfIterations, done)
		results := receiver(t, sut.C, done)

		So(results, ShouldNotBeNil)
		So(len(results), ShouldBeGreaterThan, 0)

		for _, r := range results {
			So(r.TsRcv, ShouldHappenWithin, histeresis_us, r.TsLastRcv)
		}
	})
}

func TestLimitedRateLimit(t *testing.T) {

	Convey("Given a RateLimiter", t, func() {

		const numberOfIterations int = 10
		const numberOfEventsPerSecond int64 = 2

		idealPeriod_us := time.Duration(time.Second.Microseconds()/numberOfEventsPerSecond) * time.Microsecond
		histeresis_us := time.Duration(idealPeriod_us/10) * time.Microsecond

		sourceCh := make(chan Event)
		sut := NewWithEPS(numberOfEventsPerSecond, sourceCh)
		sut.Run()

		done := make(chan struct{})
		go sender(t, sourceCh, numberOfIterations, done)
		results := receiver(t, sut.C, done)

		So(results, ShouldNotBeNil)
		So(len(results), ShouldBeGreaterThan, 0)

		for _, r := range results {
			So(r.TsRcv, ShouldHappenWithin, histeresis_us, r.TsLastRcv.Add(idealPeriod_us))
		}
	})
}

func sender(t *testing.T, sourceCh chan<- Event, numberOfEvents int, syncCh chan<- struct{}) {

	for idx := 0; idx < numberOfEvents; idx++ {
		t.Log("Sending:", idx)
		sourceCh <- Event{Timestamp: time.Now(), Tag: idx}
	}
	syncCh <- struct{}{}
}

type result struct {
	TsLastRcv time.Time
	TsRcv     time.Time
}

func receiver(t *testing.T, readCh <-chan Event, sync <-chan struct{}) []result {
	var results []result

	tsLastRcv := time.Now()
	finish := false
	for !finish {
		select {
		case ev := <-readCh:
			r := result{TsLastRcv: tsLastRcv, TsRcv: time.Now()}
			tsLastRcv = time.Now()
			t.Log(fmt.Sprintf("Time Duration %v:%v\n", ev.Tag, time.Since(r.TsLastRcv)))
			results = append(results, r)
		case <-sync:
			finish = true
		}
	}

	return results[1:]
}
