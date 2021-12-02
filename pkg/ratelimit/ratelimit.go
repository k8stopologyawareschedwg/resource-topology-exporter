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
	"time"

	"go.uber.org/ratelimit"
)

type Event struct {
	Timestamp time.Time
	Tag       int
}

type RateLimiter struct {
	outputCh     chan Event
	src          <-chan Event
	rt           ratelimit.Limiter
	ch           chan Event
	doneSender   chan struct{}
	doneReceiver chan struct{}
}

func NewWithEPS(eventsPerSecond int64, src <-chan Event) *RateLimiter {
	rt := newCommon(src)
	rt.rt = ratelimit.New(int(eventsPerSecond))
	return rt
}

func NewUnlimited(src <-chan Event) *RateLimiter {
	rt := newCommon(src)
	rt.rt = ratelimit.NewUnlimited()
	return rt
}

func newCommon(src <-chan Event) *RateLimiter {
	return &RateLimiter{
		src:          src,
		ch:           make(chan Event),
		outputCh:     make(chan Event),
		doneSender:   make(chan struct{}),
		doneReceiver: make(chan struct{}),
	}
}

func (rt *RateLimiter) Run() {

	sender := func() {
		for {
			select {
			case ev := <-rt.ch:
				rt.rt.Take()
				rt.outputCh <- ev
			case <-rt.doneSender:
				return
			}
		}
	}

	receiver := func() {
		for {
			event := <-rt.src
			select {
			case rt.ch <- event:
			case <-rt.doneReceiver:
				return
			default:
			}
		}
	}

	go sender()
	go receiver()
}

func (rt *RateLimiter) Stop() {
	rt.doneReceiver <- struct{}{}
	rt.doneSender <- struct{}{}
}

func (rt RateLimiter) OuputChannel() <-chan Event {
	return rt.outputCh
}
