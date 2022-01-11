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
	"time"

	"go.uber.org/ratelimit"

	"github.com/k8stopologyawareschedwg/resource-topology-exporter/pkg/notification"
)

//Size of the buffered channel used to handle events
// TODO: Should it be an input parameter?
const bufferSize uint16 = 5

// The ratelimiter is a kind of man-in-the-middle.
// It provides an input interface with a channel for a writter to send events at any rate
// and an ouput interface with another channel where a reader will receive events at a configured rate
// At the input interface the main goals are:
// - writer should not block at all or tha minimum time possible.
// - If there is no more room for events in the "bucket" the write should not block at all but "fail" silently.
// At the output interface the reader should get the events not faster than the configured rate, blocking if there is no event to read.
type RateLimitedEventSource struct {
	es       notification.EventSource
	inCh     <-chan notification.Event
	bufferCh chan notification.Event
	outCh    chan notification.Event
	rt       ratelimit.Limiter

	doneSenderCh   chan struct{}
	doneReceiverCh chan struct{}
	stopSenderCh   chan struct{}
	stopReceiverCh chan struct{}
}

func NewRateLimitedEventSource(es notification.EventSource, maxEventsPerTimeUnit uint64, timeUnit time.Duration) (*RateLimitedEventSource, error) {

	rles := RateLimitedEventSource{
		es:       es,
		inCh:     es.Events(),
		bufferCh: make(chan notification.Event, bufferSize),
		outCh:    make(chan notification.Event),

		doneSenderCh:   make(chan struct{}, 1),
		doneReceiverCh: make(chan struct{}, 1),
		stopSenderCh:   make(chan struct{}),
		stopReceiverCh: make(chan struct{}),
	}

	options := ratelimit.Per(timeUnit)
	rles.rt = ratelimit.New(int(maxEventsPerTimeUnit), options)

	return &rles, nil
}

func (rles *RateLimitedEventSource) Events() <-chan notification.Event {
	return rles.outCh
}

func (rles *RateLimitedEventSource) Run() {
	rles.run()
	rles.es.Run()
}

func (rles *RateLimitedEventSource) Stop() {
	//Attention: First stop the source and then the RateLimiter!
	// otherwise source could block trying to write on input channel
	// with nobody reading from it.
	rles.es.Stop()
	rles.es.Wait()
	rles.stop()
}

func (rles *RateLimitedEventSource) Wait() {
	rles.wait()
}

func (rles *RateLimitedEventSource) Close() {
	//nothing to do here, just call decorated Close
	rles.es.Close()
}

// run launch two different goroutines to avoid decorated event source
// to block on writting an event while the other one delivers events
// at the configured rate.
// see: receiver and sender functions for more info
func (rles *RateLimitedEventSource) run() {
	go rles.sender()
	go rles.receiver()
}

// receiver read from the input channel and move the event to bufferCh as fast as possible
//so it could be available to read again an so minimize the amount of time the
//decorated EventSource is blocked trying to write a new event in the "input" channel.
// Also the write in bufferCh is done so if it is full the operation silently "fails"
//instead of block
func (rles *RateLimitedEventSource) receiver() {
	for {
		select {
		case incomingEvent := <-rles.inCh:
			select {
			case rles.bufferCh <- incomingEvent:
			default:
			}
		case <-rles.stopReceiverCh:
			rles.doneReceiverCh <- struct{}{}
			return
		}
	}
}

// sender read events from the bufferCh and write it in the "output" channel at the configured rate.
func (rles *RateLimitedEventSource) sender() {
	for {
		select {
		case event := <-rles.bufferCh:
			rles.rt.Take()
			rles.outCh <- event
		case <-rles.stopSenderCh:
			rles.doneSenderCh <- struct{}{}
			return
		}
	}

}

// wait stops the caller until the EventSource is exhausted
func (rles *RateLimitedEventSource) wait() {
	<-rles.doneReceiverCh
	<-rles.doneSenderCh
}

func (rles *RateLimitedEventSource) stop() {
	rles.stopReceiverCh <- struct{}{}
	rles.stopSenderCh <- struct{}{}
}
