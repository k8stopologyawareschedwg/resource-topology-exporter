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
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUnlimitedRateLimit(t *testing.T) {

	Convey("Given a RateLimiter with no limitation", t, func() {

		const numberOfIterations int = 10
		sourceCh := make(chan Event)
		sut := NewUnlimited(sourceCh)
		sut.Run()
		done := make(chan struct{})

		Convey("When send some  messages", func() {
			go sender(t, sourceCh, numberOfIterations, done)
			results := receiver(t, sut.C, done)

			Convey("Then all messages should be received with no delay", func() {
				So(results, ShouldNotBeNil)
				So(len(results), ShouldBeGreaterThan, 0)

				histeresis_us := 70 * time.Microsecond
				for _, r := range results {
					So(r.TsRcv, ShouldHappenWithin, histeresis_us, r.TsLastRcv)
				}
			})
		})
	})
}

func TestLimitedRateLimit(t *testing.T) {

	Convey("Given a RateLimiter", t, func() {

		const numberOfIterations int = 10
		const numberOfEvents int64 = 2
		const timeUnit = 500 * time.Millisecond
		const histeresisPercentage = 10.0

		So(timeUnit, ShouldBeGreaterThanOrEqualTo, time.Microsecond)

		sourceCh := make(chan Event)
		sut := NewWithEPS(numberOfEvents, timeUnit, sourceCh)
		sut.Run()

		done := make(chan struct{})

		Convey("When some messages are send", func() {
			go sender(t, sourceCh, numberOfIterations, done)
			results := receiver(t, sut.C, done)

			So(results, ShouldNotBeNil)
			So(len(results), ShouldBeGreaterThan, 0)

			Convey("Then no more messages per time unit than the configured should be received", func() {
				idealPeriod_us := time.Duration(timeUnit.Microseconds()/numberOfEvents) * time.Microsecond
				histeresis_us := time.Duration(idealPeriod_us * time.Duration(math.Round(1-histeresisPercentage/100)))

				for _, r := range results {
					t.Logf("Ti:%v / Tf:%v / diff:%v", r.TsLastRcv, r.TsRcv, r.TsRcv.Sub(r.TsLastRcv))
					So(r.TsRcv, ShouldHappenWithin, histeresis_us, r.TsLastRcv.Add(idealPeriod_us))
				}
			})
		})
	})
}

func sender(t *testing.T, sourceCh chan<- Event, numberOfEvents int, syncCh chan<- struct{}) {

	for idx := 0; idx < numberOfEvents; idx++ {
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
		case <-readCh:
			r := result{TsLastRcv: tsLastRcv, TsRcv: time.Now()}
			tsLastRcv = time.Now()
			results = append(results, r)
		case <-sync:
			finish = true
		}
	}

	return results[1:]
}
