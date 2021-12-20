/*
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
package ratelimiter

import (
	"testing"
	"time"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/notification"
)

// Implements EventSource. Will generate an Event each "PeriodicEvents" time
type DummyEventSource struct {
	Ech            chan notification.Event
	PeriodicEvents time.Duration

	doneChan chan struct{}
	stopChan chan struct{}
}

func (es *DummyEventSource) Events() <-chan notification.Event {
	return es.Ech
}

func (es *DummyEventSource) Close() {
	//nothing to do
}

func (es *DummyEventSource) Wait() {
	<-es.doneChan
}

func (es *DummyEventSource) Stop() {
	es.stopChan <- struct{}{}
}

func (es *DummyEventSource) Run() {

	ticker := time.NewTicker(es.PeriodicEvents)

	keepLooping := true
	for keepLooping {
		select {
		case <-es.stopChan:
			keepLooping = false
		case <-ticker.C:
			es.Ech <- notification.Event{Timestamp: time.Now()}
		}
	}
	es.doneChan <- struct{}{}
}

func TestLimitedRateLimit(t *testing.T) {

	// rate limiter parameters
	const numberOfEvents int64 = 1
	const timeUnit = 500 * time.Millisecond

	// histeresis for result testing
	const histeresisPercentage = 10.0

	// EventSource
	es := DummyEventSource{
		Ech:            make(chan notification.Event),
		PeriodicEvents: 200 * time.Millisecond,

		doneChan: make(chan struct{}, 1),
		stopChan: make(chan struct{}),
	}

	// Apply RateLimit to EventSource
	sut, err := NewRateLimitedEventSource(&es, uint64(numberOfEvents), timeUnit)
	if err != nil {
		t.Fatalf("Unable to create RateLimit resource %v", err)
	}

	// Launch both EventSource and a receiver
	go sut.Run()

	done := make(chan struct{})
	var results []result
	go receiver(t, sut.Events(), done, &results)

	time.Sleep(1 * time.Second)

	// Stop both EventSource and receiver
	sut.Stop()
	sut.Wait()
	done <- struct{}{}

	// Check results
	if len(results) == 0 {
		t.Fatalf("Unable to receive results")
	}

	idealPeriodMicroseconds := time.Duration(timeUnit.Microseconds()/numberOfEvents) * time.Microsecond
	hysteresisMicroseconds := time.Duration((idealPeriodMicroseconds.Microseconds()*histeresisPercentage)/100) * time.Microsecond

	var errors []timeError
	for idx, r := range results {
		if dt, ok := WithinDuration(r.TsLastRcv.Add(idealPeriodMicroseconds), r.TsRcv, hysteresisMicroseconds); !ok {
			errors = append(errors, timeError{
				Result:     r,
				Index:      idx,
				Delta:      dt,
				Hysteresis: hysteresisMicroseconds,
			})
		}
	}

	if len(errors) != 0 {
		for _, e := range errors {
			t.Logf("Error in result: %v", e)
		}
		t.Fatal("Errors detected\n")

	}
}

func WithinDuration(expected, actual time.Time, delta time.Duration) (time.Duration, bool) {
	dt := expected.Sub(actual)
	return dt, dt >= -delta && dt <= delta
}

type result struct {
	TsLastRcv time.Time
	TsRcv     time.Time
}

type timeError struct {
	Result     result
	Delta      time.Duration
	Hysteresis time.Duration
	Index      int
}

func receiver(t *testing.T, readCh <-chan notification.Event, sync <-chan struct{}, results *[]result) {
	tsLastRcv := time.Now()
	finish := false
	for !finish {
		select {
		case <-readCh:
			r := result{TsLastRcv: tsLastRcv, TsRcv: time.Now()}
			tsLastRcv = time.Now()
			*results = append(*results, r)
		case <-sync:
			finish = true
		}
	}

	//This test tries to ensure always there is the same amount of time between output events to fulfill the rate condition.
	//The first event came out as soon as it is in the channel so the first period will be always "wrong" so to say.
	//That's why I skip the first one.
	*results = (*results)[1:]
}
